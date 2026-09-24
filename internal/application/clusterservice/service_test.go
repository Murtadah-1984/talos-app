package clusterservice

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/integrations/argocd"
)

// fakeClusterRepo is a minimal in-memory cluster.Repository covering only
// what Rollback touches (Get).
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
func (r *fakeClusterRepo) GetByName(context.Context, shared.ID, string) (*cluster.Cluster, error) {
	return nil, shared.ErrNotFound
}
func (r *fakeClusterRepo) List(context.Context, cluster.Filter, shared.Page) ([]*cluster.Cluster, error) {
	return nil, nil
}
func (r *fakeClusterRepo) Update(_ context.Context, c *cluster.Cluster) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusters[c.ID] = c
	return nil
}
func (r *fakeClusterRepo) Delete(context.Context, shared.ID) error { return nil }

// fakeGitOpsRepo is a minimal in-memory gitops.Repositories covering only
// what Rollback touches: one GitOpsConfiguration/GitRepository per cluster,
// and change set create/update/get.
type fakeGitOpsRepo struct {
	mu         sync.Mutex
	repos      map[shared.ID]*gitops.GitRepository
	configs    map[shared.ID]*gitops.GitOpsConfiguration
	changeSets map[shared.ID]*gitops.ChangeSet
}

func newFakeGitOpsRepo() *fakeGitOpsRepo {
	return &fakeGitOpsRepo{
		repos:      make(map[shared.ID]*gitops.GitRepository),
		configs:    make(map[shared.ID]*gitops.GitOpsConfiguration),
		changeSets: make(map[shared.ID]*gitops.ChangeSet),
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
	f.changeSets[c.ID] = c
	return nil
}
func (f *fakeGitOpsRepo) UpdateChangeSet(_ context.Context, c *gitops.ChangeSet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.changeSets[c.ID]; !ok {
		return shared.ErrNotFound
	}
	f.changeSets[c.ID] = c
	return nil
}
func (f *fakeGitOpsRepo) GetChangeSet(_ context.Context, id shared.ID) (*gitops.ChangeSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changeSets[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return c, nil
}
func (f *fakeGitOpsRepo) GetChangeSetByPullRequestURL(_ context.Context, url string) (*gitops.ChangeSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.changeSets {
		if c.PullRequestURL == url {
			return c, nil
		}
	}
	return nil, shared.ErrNotFound
}
func (f *fakeGitOpsRepo) ListChangeSetsForCluster(context.Context, shared.ID, shared.Page) ([]*gitops.ChangeSet, error) {
	return nil, nil
}
func (f *fakeGitOpsRepo) UpsertArgoApplication(context.Context, *gitops.ArgoApplication) error {
	return nil
}
func (f *fakeGitOpsRepo) ListArgoApplicationsForCluster(context.Context, shared.ID) ([]*gitops.ArgoApplication, error) {
	return nil, nil
}

// fakeGitProvider is a ports.GitProvider stand-in with real (if simplified)
// commit-history and per-ref file-content tracking, so Rollback's
// parent-resolution and prior-content-restoration logic can actually be
// exercised — unlike github.MockProvider, whose GetFile always returns
// ErrNotFound (it exists only to unblock the commit/PR/merge steps that
// don't read prior content).
type fakeGitProvider struct {
	mu       sync.Mutex
	branches map[string]string            // "owner/repo/branch" -> current SHA
	parents  map[string]string            // SHA -> parent SHA
	files    map[string]map[string][]byte // SHA -> path -> content (deleted paths absent)
	prs      map[int]*ports.PullRequest
	shaSeq   int
	prSeq    int
}

func newFakeGitProvider() *fakeGitProvider {
	return &fakeGitProvider{
		branches: make(map[string]string),
		parents:  make(map[string]string),
		files:    make(map[string]map[string][]byte),
		prs:      make(map[int]*ports.PullRequest),
	}
}

func branchKey(owner, repo, branch string) string { return owner + "/" + repo + "/" + branch }

func (g *fakeGitProvider) CreateBranch(_ context.Context, owner, repo, branch, fromRef string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.branches[branchKey(owner, repo, branch)] = g.branches[branchKey(owner, repo, fromRef)]
	return nil
}

func (g *fakeGitProvider) GetFile(_ context.Context, _, _, ref, path string) ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	content, ok := g.files[ref][path]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return content, nil
}

func (g *fakeGitProvider) Commit(_ context.Context, req ports.CommitRequest) (ports.CommitResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := branchKey(req.Owner, req.Repo, req.Branch)
	parent := g.branches[key]

	g.shaSeq++
	sha := fmt.Sprintf("sha-%d", g.shaSeq)
	if parent != "" {
		g.parents[sha] = parent
	}

	tree := make(map[string][]byte)
	for path, content := range g.files[parent] {
		tree[path] = content
	}
	for _, f := range req.Files {
		if f.Delete {
			delete(tree, f.Path)
			continue
		}
		tree[f.Path] = f.Content
	}
	g.files[sha] = tree
	g.branches[key] = sha
	return ports.CommitResult{SHA: sha}, nil
}

func (g *fakeGitProvider) GetCommitParent(_ context.Context, _, _, sha string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	parent, ok := g.parents[sha]
	if !ok {
		return "", shared.ErrNotFound
	}
	return parent, nil
}

func (g *fakeGitProvider) CreatePullRequest(_ context.Context, req ports.PullRequestRequest) (ports.PullRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prSeq++
	pr := &ports.PullRequest{Number: g.prSeq, URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", req.Owner, req.Repo, g.prSeq), State: "open", Head: req.Head, Base: req.Base}
	g.prs[pr.Number] = pr
	return *pr, nil
}

func (g *fakeGitProvider) GetPullRequest(_ context.Context, _, _ string, number int) (ports.PullRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	pr, ok := g.prs[number]
	if !ok {
		return ports.PullRequest{}, shared.ErrNotFound
	}
	return *pr, nil
}

func (g *fakeGitProvider) MergePullRequest(_ context.Context, _, _ string, number int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	pr, ok := g.prs[number]
	if !ok {
		return shared.ErrNotFound
	}
	pr.State = "merged"
	return nil
}

func (g *fakeGitProvider) Capability() shared.CapabilityState { return shared.CapabilityAvailable }

func TestService_ScaffoldRepository_CommitsTopLevelLayout(t *testing.T) {
	ctx := context.Background()
	git := newFakeGitProvider()

	// A repository needs at least one commit on its default branch before
	// Commit can resolve a base tree to build from — mirrors a freshly
	// created GitHub repo initialized with a README.
	if _, err := git.Commit(ctx, ports.CommitRequest{
		Owner: "acme", Repo: "gitops", Branch: "main",
		Files: []ports.FileChange{{Path: "README.md", Content: []byte("# gitops\n")}},
	}); err != nil {
		t.Fatalf("seeding initial commit: %v", err)
	}

	gitopsRepo := newFakeGitOpsRepo()
	repo := &gitops.GitRepository{ID: shared.NewID(), Owner: "acme", Name: "gitops", DefaultBranch: "main"}
	if err := gitopsRepo.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("seeding repository: %v", err)
	}

	svc := New(newFakeClusterRepo(), gitopsRepo, nil, argocd.NewMockClient(), git)

	if err := svc.ScaffoldRepository(ctx, repo.ID); err != nil {
		t.Fatalf("ScaffoldRepository: %v", err)
	}

	tree := git.files[git.branches[branchKey("acme", "gitops", "main")]]
	for _, want := range []string{"README.md", "app-of-apps.yaml", "infrastructure/README.md", "applications/README.md"} {
		if _, ok := tree[want]; !ok {
			t.Errorf("expected %s to be committed, tree has: %v", want, tree)
		}
	}
}

func TestService_Rollback_RestoresPriorContentAndDeletesNewFiles(t *testing.T) {
	ctx := context.Background()
	git := newFakeGitProvider()

	// Base commit on "main": a.yaml only.
	base, err := git.Commit(ctx, ports.CommitRequest{
		Owner: "acme", Repo: "infra", Branch: "main",
		Files: []ports.FileChange{{Path: "a.yaml", Content: []byte("v1")}},
	})
	if err != nil {
		t.Fatalf("base commit: %v", err)
	}

	// The change set under test: modifies a.yaml and adds b.yaml.
	changeCommit, err := git.Commit(ctx, ports.CommitRequest{
		Owner: "acme", Repo: "infra", Branch: "main",
		Files: []ports.FileChange{{Path: "a.yaml", Content: []byte("v2")}, {Path: "b.yaml", Content: []byte("new")}},
	})
	if err != nil {
		t.Fatalf("change commit: %v", err)
	}
	if base.SHA == changeCommit.SHA {
		t.Fatal("expected distinct SHAs")
	}

	clusters := newFakeClusterRepo()
	c := &cluster.Cluster{ID: shared.NewID(), Name: "prod"}
	c.Spec.ArgoCD.Enabled = true
	if err := clusters.Create(ctx, c); err != nil {
		t.Fatalf("seeding cluster: %v", err)
	}

	gitopsRepo := newFakeGitOpsRepo()
	repo := &gitops.GitRepository{ID: shared.NewID(), Owner: "acme", Name: "infra", DefaultBranch: "main"}
	if err := gitopsRepo.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("seeding repository: %v", err)
	}
	if err := gitopsRepo.CreateGitOpsConfiguration(ctx, &gitops.GitOpsConfiguration{ClusterID: c.ID, RepositoryID: repo.ID, Path: "clusters/prod"}); err != nil {
		t.Fatalf("seeding gitops configuration: %v", err)
	}

	target := &gitops.ChangeSet{
		ClusterID:      c.ID,
		Description:    "Upgrade cluster prod",
		GeneratedFiles: []string{"a.yaml", "b.yaml"},
		CommitSHA:      changeCommit.SHA,
		Status:         gitops.ChangeSetSynced,
	}
	if err := gitopsRepo.CreateChangeSet(ctx, target); err != nil {
		t.Fatalf("seeding change set: %v", err)
	}

	argoClient := argocd.NewMockClient()
	argoClient.Seed(c.Name, "default", "default", changeCommit.SHA)
	svc := New(clusters, gitopsRepo, nil, argoClient, git)

	revert, err := svc.Rollback(ctx, c.ID, target.ID)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if revert.Status != gitops.ChangeSetSynced {
		t.Errorf("expected reverted change set to be Synced, got %s", revert.Status)
	}
	if revert.CommitSHA == "" || revert.CommitSHA == changeCommit.SHA {
		t.Errorf("expected a new commit SHA, got %q", revert.CommitSHA)
	}

	tree := git.files[revert.CommitSHA]
	if got := string(tree["a.yaml"]); got != "v1" {
		t.Errorf("expected a.yaml restored to %q, got %q", "v1", got)
	}
	if _, exists := tree["b.yaml"]; exists {
		t.Errorf("expected b.yaml (introduced by the reverted change set) to be deleted, but it's still present")
	}
}

func TestService_Rollback_RejectsChangeSetWithoutCommitSHA(t *testing.T) {
	ctx := context.Background()
	clusters := newFakeClusterRepo()
	c := &cluster.Cluster{ID: shared.NewID(), Name: "prod"}
	if err := clusters.Create(ctx, c); err != nil {
		t.Fatalf("seeding cluster: %v", err)
	}
	gitopsRepo := newFakeGitOpsRepo()
	target := &gitops.ChangeSet{ClusterID: c.ID, Description: "no commit recorded"}
	if err := gitopsRepo.CreateChangeSet(ctx, target); err != nil {
		t.Fatalf("seeding change set: %v", err)
	}

	svc := New(clusters, gitopsRepo, nil, argocd.NewMockClient(), newFakeGitProvider())
	if _, err := svc.Rollback(ctx, c.ID, target.ID); err == nil {
		t.Fatal("expected an error for a change set with no recorded commit SHA")
	}
}
