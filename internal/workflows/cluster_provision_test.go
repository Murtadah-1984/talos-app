package workflows_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/machine"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
	"github.com/talos-platform/talos-platform/internal/infrastructure/inprocess"
	"github.com/talos-platform/talos-platform/internal/infrastructure/secrets"
	"github.com/talos-platform/talos-platform/internal/integrations/argocd"
	"github.com/talos-platform/talos-platform/internal/integrations/clusterapi"
	mockgithub "github.com/talos-platform/talos-platform/internal/integrations/github"
	mocktalos "github.com/talos-platform/talos-platform/internal/integrations/talos"
	"github.com/talos-platform/talos-platform/internal/workflows"
)

// fakeClusterRepo is a minimal in-memory cluster.Repository for exercising
// the provisioning workflow end-to-end without Postgres.
type fakeClusterRepo struct {
	mu       sync.Mutex
	clusters map[shared.ID]*cluster.Cluster
}

func newFakeClusterRepo() *fakeClusterRepo {
	return &fakeClusterRepo{clusters: make(map[shared.ID]*cluster.Cluster)}
}

func (r *fakeClusterRepo) Create(_ context.Context, c *cluster.Cluster) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusters[c.ID] = c
	return nil
}
func (r *fakeClusterRepo) Get(_ context.Context, id shared.ID) (*cluster.Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clusters[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return c, nil
}
func (r *fakeClusterRepo) GetByName(_ context.Context, _ shared.ID, name string) (*cluster.Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.clusters {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, shared.ErrNotFound
}
func (r *fakeClusterRepo) List(_ context.Context, _ cluster.Filter, _ shared.Page) ([]*cluster.Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*cluster.Cluster
	for _, c := range r.clusters {
		out = append(out, c)
	}
	return out, nil
}
func (r *fakeClusterRepo) Update(_ context.Context, c *cluster.Cluster) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusters[c.ID] = c
	return nil
}
func (r *fakeClusterRepo) Delete(_ context.Context, id shared.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clusters, id)
	return nil
}

// fakeMachineRepo is a minimal in-memory machine.Repository; List always
// returns no machines, matching a cluster with no infrastructure
// provisioned yet (Phase 6 territory), so checkTalosHealth is a no-op pass.
type fakeMachineRepo struct{ machine.Repository }

func (fakeMachineRepo) List(_ context.Context, _ machine.Filter, _ shared.Page) ([]*machine.Machine, error) {
	return nil, nil
}

// fakeMachineRepoWithMachines is a minimal in-memory machine.Repository that
// actually lists seeded machines, filtered by ClusterID — unlike
// fakeMachineRepo, for tests exercising checkTalosHealth's per-machine
// behavior instead of treating it as a no-op.
type fakeMachineRepoWithMachines struct {
	machine.Repository
	machines []*machine.Machine
}

func (r fakeMachineRepoWithMachines) List(_ context.Context, filter machine.Filter, _ shared.Page) ([]*machine.Machine, error) {
	if filter.ClusterID == nil {
		return r.machines, nil
	}
	var out []*machine.Machine
	for _, m := range r.machines {
		if m.ClusterID != nil && *m.ClusterID == *filter.ClusterID {
			out = append(out, m)
		}
	}
	return out, nil
}

// fakeGitOpsRepo is a minimal in-memory gitops.Repositories covering only
// what the provisioning workflow touches: one GitOpsConfiguration/
// GitRepository per cluster, and change set create/update/list.
type fakeGitOpsRepo struct {
	mu         sync.Mutex
	repos      map[shared.ID]*gitops.GitRepository
	configs    map[shared.ID]*gitops.GitOpsConfiguration // by cluster ID
	changeSets map[shared.ID][]*gitops.ChangeSet         // by cluster ID, newest first
	argoApps   map[shared.ID][]*gitops.ArgoApplication
}

func newFakeGitOpsRepo() *fakeGitOpsRepo {
	return &fakeGitOpsRepo{
		repos:      make(map[shared.ID]*gitops.GitRepository),
		configs:    make(map[shared.ID]*gitops.GitOpsConfiguration),
		changeSets: make(map[shared.ID][]*gitops.ChangeSet),
		argoApps:   make(map[shared.ID][]*gitops.ArgoApplication),
	}
}

func (f *fakeGitOpsRepo) CreateGitProvider(context.Context, *gitops.GitProviderConfig) error {
	return nil
}
func (f *fakeGitOpsRepo) GetGitProvider(context.Context, shared.ID) (*gitops.GitProviderConfig, error) {
	return nil, shared.ErrNotFound
}
func (f *fakeGitOpsRepo) ListGitProviders(context.Context, shared.ID) ([]*gitops.GitProviderConfig, error) {
	return nil, nil
}

func (f *fakeGitOpsRepo) CreateRepository(_ context.Context, r *gitops.GitRepository) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.ID == (shared.ID{}) {
		r.ID = shared.NewID()
	}
	f.repos[r.ID] = r
	return nil
}
func (f *fakeGitOpsRepo) GetRepository(_ context.Context, id shared.ID) (*gitops.GitRepository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.repos[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return r, nil
}
func (f *fakeGitOpsRepo) ListRepositories(context.Context, shared.ID) ([]*gitops.GitRepository, error) {
	return nil, nil
}

func (f *fakeGitOpsRepo) CreateGitOpsConfiguration(_ context.Context, c *gitops.GitOpsConfiguration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.configs[c.ClusterID] = c
	return nil
}
func (f *fakeGitOpsRepo) GetGitOpsConfigurationForCluster(_ context.Context, clusterID shared.ID) (*gitops.GitOpsConfiguration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.configs[clusterID]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return c, nil
}

func (f *fakeGitOpsRepo) CreateChangeSet(_ context.Context, c *gitops.ChangeSet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c.ID == (shared.ID{}) {
		c.ID = shared.NewID()
	}
	// Prepend so index 0 is always the most recent, matching the real
	// repository's ORDER BY created_at DESC.
	f.changeSets[c.ClusterID] = append([]*gitops.ChangeSet{c}, f.changeSets[c.ClusterID]...)
	return nil
}
func (f *fakeGitOpsRepo) UpdateChangeSet(_ context.Context, c *gitops.ChangeSet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.changeSets[c.ClusterID] {
		if existing.ID == c.ID {
			*existing = *c
		}
	}
	return nil
}
func (f *fakeGitOpsRepo) GetChangeSet(_ context.Context, id shared.ID) (*gitops.ChangeSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, sets := range f.changeSets {
		for _, existing := range sets {
			if existing.ID == id {
				return existing, nil
			}
		}
	}
	return nil, shared.ErrNotFound
}
func (f *fakeGitOpsRepo) GetChangeSetByPullRequestURL(_ context.Context, url string) (*gitops.ChangeSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, sets := range f.changeSets {
		for _, existing := range sets {
			if existing.PullRequestURL == url {
				return existing, nil
			}
		}
	}
	return nil, shared.ErrNotFound
}
func (f *fakeGitOpsRepo) ListChangeSetsForCluster(_ context.Context, clusterID shared.ID, page shared.Page) ([]*gitops.ChangeSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.changeSets[clusterID]
	if page.Limit > 0 && page.Limit < len(all) {
		return all[:page.Limit], nil
	}
	return all, nil
}

func (f *fakeGitOpsRepo) UpsertArgoApplication(_ context.Context, a *gitops.ArgoApplication) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.argoApps[a.ClusterID] = append(f.argoApps[a.ClusterID], a)
	return nil
}
func (f *fakeGitOpsRepo) ListArgoApplicationsForCluster(_ context.Context, clusterID shared.ID) ([]*gitops.ArgoApplication, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.argoApps[clusterID], nil
}

var _ gitops.Repositories = (*fakeGitOpsRepo)(nil)

// TestClusterProvisionWorkflow_EndToEnd exercises the full §4 pipeline —
// cluster request -> Git commit -> PR -> merge -> Argo CD sync -> READY —
// against the mock Git/Argo CD/Cluster API/Talos adapters, using the real
// workflow engine and gitopsrender package. This is the "complete cluster
// request -> Git commit -> PR -> merge -> Argo CD" test called for by the
// Phase 3 roadmap item.
func TestClusterProvisionWorkflow_EndToEnd(t *testing.T) {
	clusters := newFakeClusterRepo()
	gitopsRepo := newFakeGitOpsRepo()
	gitProvider := mockgithub.NewMockProvider()
	argoClient := argocd.NewMockClient()
	capiProvider := clusterapi.NewMockProvider()
	talosClient := mocktalos.NewMockClient()

	repo := &gitops.GitRepository{ID: shared.NewID(), Owner: "acme", Name: "gitops", DefaultBranch: "main"}
	if err := gitopsRepo.CreateRepository(context.Background(), repo); err != nil {
		t.Fatalf("seeding git repository: %v", err)
	}

	c := &cluster.Cluster{
		ID:           shared.NewID(),
		SiteID:       shared.NewID(),
		Name:         "basra-prod",
		ProviderMode: cluster.ProviderModeDirectTalos,
		State:        cluster.StateProvisioning,
		Spec: cluster.Spec{
			KubernetesVersion: "v1.31.1",
			TalosVersion:      "v1.8.2",
			ControlPlane:      cluster.ControlPlaneSpec{Replicas: 3},
			Workers:           []cluster.WorkerPoolSpec{{Name: "default", Replicas: 3}},
			Network:           cluster.NetworkSpec{PodCIDR: "10.244.0.0/16", ServiceCIDR: "10.96.0.0/12"},
			ArgoCD:            cluster.ArgoCDSpec{Enabled: true, Project: "default"},
		},
	}
	if err := clusters.Create(context.Background(), c); err != nil {
		t.Fatalf("seeding cluster: %v", err)
	}
	if err := gitopsRepo.CreateGitOpsConfiguration(context.Background(), &gitops.GitOpsConfiguration{
		ClusterID: c.ID, RepositoryID: repo.ID, Path: "clusters/basra-prod", Branch: repo.DefaultBranch,
	}); err != nil {
		t.Fatalf("seeding gitops configuration: %v", err)
	}
	// Argo CD only reports status for Applications it already knows about;
	// seed it the way a real Argo CD instance would have picked up the
	// Application manifest committed by this workflow.
	argoClient.Seed(c.Name, "default", "default", "abc123")

	deps := workflows.ClusterProvisionDeps{
		Clusters: clusters, Machines: fakeMachineRepo{}, GitOps: gitopsRepo,
		Talos: talosClient, Git: gitProvider, ArgoCD: argoClient, ClusterAPI: capiProvider,
	}

	broker := inprocess.NewBroker()
	workflowRepo := newFakeRepo()
	engine := workflows.NewEngine(workflowRepo, broker, nil)
	engine.Register(workflows.NewClusterProvisionDefinition(deps))
	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	wf, err := engine.Enqueue(context.Background(), workflow.TypeClusterProvision, "e2e-test", nil, &c.ID, nil, shared.NewID())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	gotWF, steps, err := engine.Get(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotWF.Status != workflow.StatusSucceeded {
		var failed string
		for _, s := range steps {
			if s.Status == workflow.StatusFailed {
				failed = s.Name + ": " + s.Error
			}
		}
		t.Fatalf("expected workflow to succeed, got %s (workflow error: %s, step error: %s)", gotWF.Status, gotWF.Error, failed)
	}

	final, err := clusters.Get(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Get cluster: %v", err)
	}
	if final.State != cluster.StateReady {
		t.Fatalf("expected cluster to reach READY, got %s", final.State)
	}

	changeSets, err := gitopsRepo.ListChangeSetsForCluster(context.Background(), c.ID, shared.DefaultPage())
	if err != nil || len(changeSets) != 1 {
		t.Fatalf("expected exactly one change set, got %d (err: %v)", len(changeSets), err)
	}
	if changeSets[0].Status != gitops.ChangeSetSynced {
		t.Fatalf("expected change set to end as SYNCED, got %s", changeSets[0].Status)
	}
	if !strings.Contains(changeSets[0].PullRequestURL, "/pull/") {
		t.Fatalf("expected a pull request URL, got %q", changeSets[0].PullRequestURL)
	}
}

// TestClusterProvisionWorkflow_RegistersPerClusterTalosCredentials exercises
// Phase 8 multi-cluster credential handling: checkTalosHealth must resolve
// the cluster's own TalosConfigRef and register it with the TalosClient
// adapter for each of the cluster's machine endpoints before checking their
// health, so one platform process can provision/manage more than one Talos
// cluster's machines.
func TestClusterProvisionWorkflow_RegistersPerClusterTalosCredentials(t *testing.T) {
	clusters := newFakeClusterRepo()
	gitopsRepo := newFakeGitOpsRepo()
	gitProvider := mockgithub.NewMockProvider()
	argoClient := argocd.NewMockClient()
	capiProvider := clusterapi.NewMockProvider()
	talosClient := mocktalos.NewMockClient()

	secretStore, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	talosconfig := []byte("context: basra-prod\ncontexts:\n  basra-prod: {}\n")
	if err := secretStore.Put(context.Background(), ports.SecretRef{Backend: "talos", Path: "basra-prod-talosconfig"}, talosconfig); err != nil {
		t.Fatalf("seeding secret store: %v", err)
	}

	repo := &gitops.GitRepository{ID: shared.NewID(), Owner: "acme", Name: "gitops", DefaultBranch: "main"}
	if err := gitopsRepo.CreateRepository(context.Background(), repo); err != nil {
		t.Fatalf("seeding git repository: %v", err)
	}

	c := &cluster.Cluster{
		ID:             shared.NewID(),
		SiteID:         shared.NewID(),
		Name:           "basra-prod",
		ProviderMode:   cluster.ProviderModeDirectTalos,
		State:          cluster.StateProvisioning,
		TalosConfigRef: "basra-prod-talosconfig",
		Spec: cluster.Spec{
			KubernetesVersion: "v1.31.1",
			TalosVersion:      "v1.8.2",
			ControlPlane:      cluster.ControlPlaneSpec{Replicas: 1},
			Network:           cluster.NetworkSpec{PodCIDR: "10.244.0.0/16", ServiceCIDR: "10.96.0.0/12"},
			ArgoCD:            cluster.ArgoCDSpec{Enabled: true, Project: "default"},
		},
	}
	if err := clusters.Create(context.Background(), c); err != nil {
		t.Fatalf("seeding cluster: %v", err)
	}
	if err := gitopsRepo.CreateGitOpsConfiguration(context.Background(), &gitops.GitOpsConfiguration{
		ClusterID: c.ID, RepositoryID: repo.ID, Path: "clusters/basra-prod", Branch: repo.DefaultBranch,
	}); err != nil {
		t.Fatalf("seeding gitops configuration: %v", err)
	}
	argoClient.Seed(c.Name, "default", "default", "abc123")

	m := &machine.Machine{ID: shared.NewID(), ClusterID: &c.ID, ManagementIP: "10.0.0.9", Hostname: "cp-1"}
	machines := fakeMachineRepoWithMachines{machines: []*machine.Machine{m}}

	deps := workflows.ClusterProvisionDeps{
		Clusters: clusters, Machines: machines, GitOps: gitopsRepo,
		Talos: talosClient, Git: gitProvider, ArgoCD: argoClient, ClusterAPI: capiProvider,
		SecretStore: secretStore,
	}

	broker := inprocess.NewBroker()
	workflowRepo := newFakeRepo()
	engine := workflows.NewEngine(workflowRepo, broker, nil)
	engine.Register(workflows.NewClusterProvisionDefinition(deps))
	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	wf, err := engine.Enqueue(context.Background(), workflow.TypeClusterProvision, "multi-cluster-creds-test", nil, &c.ID, nil, shared.NewID())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	gotWF, steps, err := engine.Get(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotWF.Status != workflow.StatusSucceeded {
		var failed string
		for _, s := range steps {
			if s.Status == workflow.StatusFailed {
				failed = s.Name + ": " + s.Error
			}
		}
		t.Fatalf("expected workflow to succeed, got %s (workflow error: %s, step error: %s)", gotWF.Status, gotWF.Error, failed)
	}

	got, ok := talosClient.CredentialsFor(m.ManagementIP)
	if !ok {
		t.Fatal("expected EnsureCredentials to have been called for the machine's endpoint")
	}
	if string(got) != string(talosconfig) {
		t.Errorf("expected the cluster's own talosconfig to be registered, got %q", got)
	}
}

// TestClusterAPIWorkflows_CommitScaleChangeToGit exercises the Phase 5
// addition: for a CLUSTER_API-mode cluster, WORKER_SCALE must commit the new
// desired state (including rendered CAPI manifests) to Git rather than
// applying anything directly (ADR-0002) — the same "commit -> PR -> merge"
// shape as provisioning, reused via commitClusterAPIChange.
func TestClusterAPIWorkflows_CommitScaleChangeToGit(t *testing.T) {
	clusters := newFakeClusterRepo()
	gitopsRepo := newFakeGitOpsRepo()
	gitProvider := mockgithub.NewMockProvider()
	argoClient := argocd.NewMockClient()
	capiProvider := clusterapi.NewMockProvider()
	talosClient := mocktalos.NewMockClient()

	repo := &gitops.GitRepository{ID: shared.NewID(), Owner: "acme", Name: "gitops", DefaultBranch: "main"}
	if err := gitopsRepo.CreateRepository(context.Background(), repo); err != nil {
		t.Fatalf("seeding git repository: %v", err)
	}

	c := &cluster.Cluster{
		ID:           shared.NewID(),
		SiteID:       shared.NewID(),
		Name:         "capi-prod",
		ProviderMode: cluster.ProviderModeClusterAPI,
		State:        cluster.StateReady,
		Spec: cluster.Spec{
			KubernetesVersion: "v1.31.1",
			TalosVersion:      "v1.8.2",
			ControlPlane:      cluster.ControlPlaneSpec{Replicas: 3},
			Workers:           []cluster.WorkerPoolSpec{{Name: "default", Replicas: 3}},
			Network:           cluster.NetworkSpec{PodCIDR: "10.244.0.0/16", ServiceCIDR: "10.96.0.0/12"},
		},
	}
	if err := clusters.Create(context.Background(), c); err != nil {
		t.Fatalf("seeding cluster: %v", err)
	}
	if err := gitopsRepo.CreateGitOpsConfiguration(context.Background(), &gitops.GitOpsConfiguration{
		ClusterID: c.ID, RepositoryID: repo.ID, Path: "clusters/capi-prod", Branch: repo.DefaultBranch,
	}); err != nil {
		t.Fatalf("seeding gitops configuration: %v", err)
	}

	deps := workflows.ClusterProvisionDeps{
		Clusters: clusters, Machines: fakeMachineRepo{}, GitOps: gitopsRepo,
		Talos: talosClient, Git: gitProvider, ArgoCD: argoClient, ClusterAPI: capiProvider,
	}

	broker := inprocess.NewBroker()
	workflowRepo := newFakeRepo()
	engine := workflows.NewEngine(workflowRepo, broker, nil)
	engine.Register(workflows.NewWorkerScaleDefinition(deps))
	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	// Simulate clusterservice.ScaleWorkers having already updated the
	// desired replica count before enqueuing the workflow.
	c.Spec.Workers[0].Replicas = 6
	if err := clusters.Update(context.Background(), c); err != nil {
		t.Fatalf("updating cluster spec: %v", err)
	}

	wf, err := engine.Enqueue(context.Background(), workflow.TypeWorkerScale, "scale-test", nil, &c.ID, nil, shared.NewID())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	gotWF, steps, err := engine.Get(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotWF.Status != workflow.StatusSucceeded {
		var failed string
		for _, s := range steps {
			if s.Status == workflow.StatusFailed {
				failed = s.Name + ": " + s.Error
			}
		}
		t.Fatalf("expected workflow to succeed, got %s (workflow error: %s, step error: %s)", gotWF.Status, gotWF.Error, failed)
	}

	changeSets, err := gitopsRepo.ListChangeSetsForCluster(context.Background(), c.ID, shared.DefaultPage())
	if err != nil || len(changeSets) != 1 {
		t.Fatalf("expected exactly one change set from the scale workflow, got %d (err: %v)", len(changeSets), err)
	}
	if changeSets[0].Status != gitops.ChangeSetMerged {
		t.Fatalf("expected the scale change set to be MERGED (no Argo CD enabled), got %s", changeSets[0].Status)
	}
}
