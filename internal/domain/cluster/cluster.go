// Package cluster models a managed Kubernetes/Talos cluster: its desired
// configuration, its provider mode, and its observed lifecycle state.
//
// A Cluster record in Postgres is metadata and a pointer into Git (ADR-0004);
// the authoritative desired configuration lives in the GitOps repository once
// the cluster has been committed. The struct below is also what the platform
// renders into GitOps manifests before that commit exists.
package cluster

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// ProviderMode selects how the cluster's lifecycle is reconciled (ADR-0002).
type ProviderMode string

const (
	ProviderModeDirectTalos ProviderMode = "DIRECT_TALOS"
	ProviderModeClusterAPI  ProviderMode = "CLUSTER_API"
)

// State is the observed lifecycle state of a cluster (§8).
type State string

const (
	StateDraft         State = "DRAFT"
	StatePlanning      State = "PLANNING"
	StateValidating    State = "VALIDATING"
	StateProvisioning  State = "PROVISIONING"
	StateBootstrapping State = "BOOTSTRAPPING"
	StateInstalling    State = "INSTALLING"
	StateConfiguring   State = "CONFIGURING"
	StateReady         State = "READY"
	StateDegraded      State = "DEGRADED"
	StateUpgrading     State = "UPGRADING"
	StateFailed        State = "FAILED"
	StateDeleting      State = "DELETING"
	StateDeleted       State = "DELETED"
)

// validTransitions enumerates the allowed state graph so the application layer
// can reject illegal jumps (e.g. DRAFT -> READY) instead of trusting callers.
var validTransitions = map[State][]State{
	StateDraft:         {StatePlanning, StateDeleting},
	StatePlanning:      {StateValidating, StateFailed, StateDeleting},
	StateValidating:    {StateProvisioning, StateFailed, StateDeleting},
	StateProvisioning:  {StateBootstrapping, StateFailed, StateDeleting},
	StateBootstrapping: {StateInstalling, StateFailed, StateDeleting},
	StateInstalling:    {StateConfiguring, StateFailed, StateDeleting},
	StateConfiguring:   {StateReady, StateFailed, StateDeleting},
	StateReady:         {StateDegraded, StateUpgrading, StateDeleting},
	StateDegraded:      {StateReady, StateFailed, StateDeleting},
	StateUpgrading:     {StateReady, StateDegraded, StateFailed},
	StateFailed:        {StatePlanning, StateDeleting},
	StateDeleting:      {StateDeleted, StateFailed},
	StateDeleted:       {},
}

// CanTransition reports whether moving from s to next is a legal lifecycle step.
func (s State) CanTransition(next State) bool {
	for _, allowed := range validTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// ControlPlaneSpec configures the control plane machine pool (§9).
type ControlPlaneSpec struct {
	Replicas int32
	CPU      int32
	MemoryGB int32
}

// WorkerPoolSpec configures a named worker machine pool (§9).
type WorkerPoolSpec struct {
	Name     string
	Replicas int32
	CPU      int32
	MemoryGB int32
	Labels   map[string]string
	Taints   []string
}

// NetworkSpec configures cluster-wide networking (§9).
type NetworkSpec struct {
	PodCIDR     string
	ServiceCIDR string
}

// CNISpec selects and configures the CNI plugin.
type CNISpec struct {
	Type   string // e.g. "calico", "cilium"
	Values map[string]string
}

// StorageSpec selects the CSI driver.
type StorageSpec struct {
	CSI    string
	Values map[string]string
}

// IngressSpec selects the ingress controller.
type IngressSpec struct {
	Controller string
	Values     map[string]string
}

// ObservabilitySpec toggles the in-cluster observability stack.
type ObservabilitySpec struct {
	Prometheus    bool
	Grafana       bool
	Loki          bool
	OpenTelemetry bool
}

// GitOpsSpec points at the GitOps repository/path this cluster is rendered into.
type GitOpsSpec struct {
	RepositoryID shared.ID
	Path         string
	Branch       string
}

// ArgoCDSpec configures whether/how Argo CD manages this cluster's applications.
type ArgoCDSpec struct {
	Enabled bool
	Project string
}

// Spec is the desired configuration of a cluster (§9). It is intentionally
// free of any provider-specific or infrastructure-specific fields — those are
// resolved via Site/InfrastructureProvider at render time.
type Spec struct {
	KubernetesVersion string
	TalosVersion      string
	ControlPlane      ControlPlaneSpec
	Workers           []WorkerPoolSpec
	Network           NetworkSpec
	CNI               CNISpec
	Storage           StorageSpec
	Ingress           IngressSpec
	Observability     ObservabilitySpec
	GitOps            GitOpsSpec
	ArgoCD            ArgoCDSpec
}

// Cluster is the aggregate root for a managed Kubernetes cluster.
type Cluster struct {
	ID             shared.ID
	OrganizationID shared.ID
	ProjectID      shared.ID
	EnvironmentID  shared.ID
	SiteID         shared.ID
	TemplateID     *shared.ID

	Name         string
	Endpoint     string
	ProviderMode ProviderMode
	State        State

	// TalosConfigRef points at this cluster's own talosconfig (PKI) in the
	// SecretStore (ADR-0001, ADR-0006), letting one platform process reach
	// more than one Talos cluster's machines — each keyed by which cluster
	// owns the target endpoint, resolved via the owning machine's
	// ClusterID. Empty means "use the process's single default talosconfig"
	// (PLATFORM_TALOS_CONFIG_FILE), the original single-cluster deployment
	// shape, which keeps working unchanged.
	TalosConfigRef string

	Spec Spec

	// GitCommitSHA is the last commit rendered/observed for this cluster's
	// GitOps path, used for drift/version comparison (ADR-0004).
	GitCommitSHA string

	shared.Timestamps
}

// Transition validates and applies a state change, returning an error if the
// transition is not legal from the cluster's current state.
func (c *Cluster) Transition(next State) error {
	if !c.State.CanTransition(next) {
		return shared.ErrPreconditionFail
	}
	c.State = next
	c.Touch()
	return nil
}

// Repository persists clusters.
type Repository interface {
	Create(ctx context.Context, c *Cluster) error
	Get(ctx context.Context, id shared.ID) (*Cluster, error)
	GetByName(ctx context.Context, orgID shared.ID, name string) (*Cluster, error)
	List(ctx context.Context, filter Filter, page shared.Page) ([]*Cluster, error)
	Update(ctx context.Context, c *Cluster) error
	Delete(ctx context.Context, id shared.ID) error
}

// Filter narrows a cluster list query (§15). Zero values mean "no filter".
type Filter struct {
	OrganizationID    *shared.ID
	ProjectID         *shared.ID
	EnvironmentID     *shared.ID
	SiteID            *shared.ID
	ProviderMode      ProviderMode
	State             State
	KubernetesVersion string
	TalosVersion      string
}
