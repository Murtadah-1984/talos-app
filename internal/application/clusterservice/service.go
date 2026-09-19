// Package clusterservice implements the cluster lifecycle use cases exposed
// by the REST API and CLI (§8, §9, §17, §27). It orchestrates the domain
// model and the workflow engine for declarative desired-state changes
// (provision/upgrade/scale/delete all go through a workflow — ADR-0001..0005).
// The one exception is TriggerSync: an explicit, imperative "sync now"
// request is not a desired-state change (Argo CD's sync policy already
// governs that), so it calls ports.ArgoCDClient directly here, the same way
// machineservice calls ports.TalosClient directly for reboot/upgrade.
package clusterservice

import (
	"context"
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
)

// WorkflowEnqueuer decouples this service from the concrete workflow engine
// type, since internal/workflows imports domain packages this service also
// depends on and Go forbids the resulting import cycle.
type WorkflowEnqueuer interface {
	Enqueue(ctx context.Context, wtype workflow.Type, idempotencyKey string, input map[string]any, clusterID, machineID *shared.ID, requestedBy shared.ID) (*workflow.Workflow, error)
}

type Service struct {
	clusters  cluster.Repository
	gitops    gitops.Repositories
	workflows WorkflowEnqueuer
	argocd    ports.ArgoCDClient
}

func New(clusters cluster.Repository, gitopsRepo gitops.Repositories, workflows WorkflowEnqueuer, argocd ports.ArgoCDClient) *Service {
	return &Service{clusters: clusters, gitops: gitopsRepo, workflows: workflows, argocd: argocd}
}

// CreateInput is the payload for creating a new cluster in DRAFT state,
// mirroring the guided creation flow in §49.
type CreateInput struct {
	OrganizationID shared.ID
	ProjectID      shared.ID
	EnvironmentID  shared.ID
	SiteID         shared.ID
	TemplateID     *shared.ID
	Name           string
	ProviderMode   cluster.ProviderMode
	Spec           cluster.Spec
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*cluster.Cluster, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("%w: cluster name is required", shared.ErrInvalidInput)
	}
	c := &cluster.Cluster{
		ID:             shared.NewID(),
		OrganizationID: in.OrganizationID,
		ProjectID:      in.ProjectID,
		EnvironmentID:  in.EnvironmentID,
		SiteID:         in.SiteID,
		TemplateID:     in.TemplateID,
		Name:           in.Name,
		ProviderMode:   in.ProviderMode,
		State:          cluster.StateDraft,
		Spec:           in.Spec,
	}
	if err := s.clusters.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("creating cluster: %w", err)
	}
	return c, nil
}

func (s *Service) Get(ctx context.Context, id shared.ID) (*cluster.Cluster, error) {
	return s.clusters.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, filter cluster.Filter, page shared.Page) ([]*cluster.Cluster, error) {
	return s.clusters.List(ctx, filter, page)
}

// PlanResult is the human-readable diff shown on the review page before
// provisioning (§49): "PLAN: + Create Cluster, + Create N Control Plane
// Machines, ...".
type PlanResult struct {
	Actions []string
}

// Plan validates the cluster's spec and renders the change summary an
// operator reviews before requesting provisioning. It performs no writes.
func (s *Service) Plan(ctx context.Context, id shared.ID) (*PlanResult, error) {
	c, err := s.clusters.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Spec.KubernetesVersion == "" || c.Spec.TalosVersion == "" {
		return nil, fmt.Errorf("%w: cluster spec is missing Kubernetes/Talos versions", shared.ErrInvalidInput)
	}

	actions := []string{fmt.Sprintf("+ Create Cluster %s (%s)", c.Name, c.ProviderMode)}
	actions = append(actions, fmt.Sprintf("+ Create %d Control Plane Machine(s)", c.Spec.ControlPlane.Replicas))
	for _, w := range c.Spec.Workers {
		actions = append(actions, fmt.Sprintf("+ Create %d Worker Machine(s) in pool %q", w.Replicas, w.Name))
	}
	if c.Spec.CNI.Type != "" {
		actions = append(actions, fmt.Sprintf("+ Install CNI: %s", c.Spec.CNI.Type))
	}
	if c.Spec.Storage.CSI != "" {
		actions = append(actions, fmt.Sprintf("+ Install CSI: %s", c.Spec.Storage.CSI))
	}
	if c.Spec.Ingress.Controller != "" {
		actions = append(actions, fmt.Sprintf("+ Install Ingress Controller: %s", c.Spec.Ingress.Controller))
	}
	if c.Spec.ArgoCD.Enabled {
		actions = append(actions, "+ Configure Argo CD GitOps")
	}
	return &PlanResult{Actions: actions}, nil
}

// Provision moves the cluster through PLANNING -> VALIDATING -> PROVISIONING
// synchronously (§8) and enqueues the durable CLUSTER_PROVISION workflow
// (§23, ADR-0005) that performs the rest of the pipeline. idempotencyKey
// lets a retried API call safely resolve to the same workflow run (§34).
func (s *Service) Provision(ctx context.Context, id shared.ID, requestedBy shared.ID, idempotencyKey string) (*workflow.Workflow, error) {
	c, err := s.clusters.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.Plan(ctx, id); err != nil {
		return nil, fmt.Errorf("plan validation failed: %w", err)
	}

	for _, next := range []cluster.State{cluster.StatePlanning, cluster.StateValidating, cluster.StateProvisioning} {
		if err := c.Transition(next); err != nil {
			return nil, fmt.Errorf("transitioning cluster to %s: %w", next, err)
		}
	}
	if err := s.clusters.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("persisting cluster state: %w", err)
	}

	wf, err := s.workflows.Enqueue(ctx, workflow.TypeClusterProvision, idempotencyKey, map[string]any{"clusterId": c.ID.String()}, &c.ID, nil, requestedBy)
	if err != nil {
		return nil, fmt.Errorf("enqueuing provisioning workflow: %w", err)
	}
	return wf, nil
}

// Delete transitions the cluster to DELETING. The actual teardown (and the
// eventual transition to DELETED) happens via the CLUSTER_DELETE workflow,
// mirroring the same "commit -> reconcile" model used for provisioning.
func (s *Service) Delete(ctx context.Context, id shared.ID, requestedBy shared.ID, idempotencyKey string) (*workflow.Workflow, error) {
	c, err := s.clusters.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := c.Transition(cluster.StateDeleting); err != nil {
		return nil, fmt.Errorf("transitioning cluster to DELETING: %w", err)
	}
	if err := s.clusters.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("persisting cluster state: %w", err)
	}
	return s.workflows.Enqueue(ctx, workflow.TypeClusterDelete, idempotencyKey, map[string]any{"clusterId": c.ID.String()}, &c.ID, nil, requestedBy)
}

// Upgrade validates the requested target versions and enqueues the durable
// CLUSTER_UPGRADE workflow (§18), which performs the actual staged
// control-plane-then-worker rollout. The cluster's Spec is updated to the
// target versions immediately so subsequent Plan/diff calls reflect the
// desired state, matching Git-is-source-of-truth once GitOps wiring lands.
func (s *Service) Upgrade(ctx context.Context, id shared.ID, requestedBy shared.ID, idempotencyKey, targetKubernetesVersion, targetTalosVersion string) (*workflow.Workflow, error) {
	c, err := s.clusters.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if targetKubernetesVersion == "" && targetTalosVersion == "" {
		return nil, fmt.Errorf("%w: at least one target version is required", shared.ErrInvalidInput)
	}
	if targetKubernetesVersion != "" {
		c.Spec.KubernetesVersion = targetKubernetesVersion
	}
	if targetTalosVersion != "" {
		c.Spec.TalosVersion = targetTalosVersion
	}
	if err := s.clusters.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("persisting target versions: %w", err)
	}

	return s.workflows.Enqueue(ctx, workflow.TypeClusterUpgrade, idempotencyKey, map[string]any{
		"clusterId":         c.ID.String(),
		"kubernetesVersion": targetKubernetesVersion,
		"talosVersion":      targetTalosVersion,
	}, &c.ID, nil, requestedBy)
}

// ScaleWorkers updates the named worker pool's replica count and enqueues
// the WORKER_SCALE workflow (§17).
func (s *Service) ScaleWorkers(ctx context.Context, id shared.ID, requestedBy shared.ID, idempotencyKey, poolName string, replicas int32) (*workflow.Workflow, error) {
	c, err := s.clusters.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range c.Spec.Workers {
		if c.Spec.Workers[i].Name == poolName {
			c.Spec.Workers[i].Replicas = replicas
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: worker pool %q not found", shared.ErrNotFound, poolName)
	}
	if err := s.clusters.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("persisting scale request: %w", err)
	}

	return s.workflows.Enqueue(ctx, workflow.TypeWorkerScale, idempotencyKey, map[string]any{
		"clusterId": c.ID.String(),
		"pool":      poolName,
		"replicas":  replicas,
	}, &c.ID, nil, requestedBy)
}

// GitOpsStatus surfaces the cached Argo CD Application status and recent
// change sets for a cluster's dashboard tab (§4, §15).
type GitOpsStatus struct {
	Applications []*gitops.ArgoApplication
	ChangeSets   []*gitops.ChangeSet
}

func (s *Service) GitOpsStatus(ctx context.Context, clusterID shared.ID) (*GitOpsStatus, error) {
	apps, err := s.gitops.ListArgoApplicationsForCluster(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("loading argo applications: %w", err)
	}
	changes, err := s.gitops.ListChangeSetsForCluster(ctx, clusterID, shared.DefaultPage())
	if err != nil {
		return nil, fmt.Errorf("loading change sets: %w", err)
	}
	return &GitOpsStatus{Applications: apps, ChangeSets: changes}, nil
}

// TriggerSync requests an immediate Argo CD sync for the cluster's
// Application (§4 "trigger synchronization where appropriate"). This is an
// imperative, operator-requested action, not a desired-state change — Argo
// CD's own sync policy (automated or manual) continues to govern steady
// state regardless.
func (s *Service) TriggerSync(ctx context.Context, clusterID shared.ID) error {
	c, err := s.clusters.Get(ctx, clusterID)
	if err != nil {
		return err
	}
	if !c.Spec.ArgoCD.Enabled {
		return fmt.Errorf("%w: cluster %s does not have Argo CD enabled", shared.ErrInvalidInput, c.Name)
	}
	if err := s.argocd.Sync(ctx, c.Name); err != nil {
		return fmt.Errorf("triggering sync for cluster %s: %w", c.Name, err)
	}
	return nil
}
