// Package github implements the ports.GitProvider adapter for GitHub
// (§19, ADR-0004). The real client (using go-github + an App/PAT credential
// resolved via SecretStore) lands in Phase 3; this package's mock lets the
// GitOps commit/PR workflow be built and tested without a live GitHub account.
package github

import (
	"context"
	"fmt"
	"sync"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// MockProvider is an in-memory GitHub stand-in: branches, commits, and PRs
// are tracked in a map rather than a real remote.
type MockProvider struct {
	mu       sync.Mutex
	branches map[string]string // "owner/repo/branch" -> latest SHA
	prs      map[string]*ports.PullRequest
	nextPR   int
	shaSeq   int
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		branches: make(map[string]string),
		prs:      make(map[string]*ports.PullRequest),
		nextPR:   1,
	}
}

func key(owner, repo, branch string) string { return owner + "/" + repo + "/" + branch }

func (p *MockProvider) CreateBranch(_ context.Context, owner, repo, branch, fromRef string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.branches[key(owner, repo, branch)] = fromRef
	return nil
}

func (p *MockProvider) GetFile(_ context.Context, owner, repo, ref, path string) ([]byte, error) {
	return nil, fmt.Errorf("%w: mock provider has no file storage for %s/%s@%s:%s", shared.ErrNotFound, owner, repo, ref, path)
}

func (p *MockProvider) Commit(_ context.Context, req ports.CommitRequest) (ports.CommitResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shaSeq++
	sha := fmt.Sprintf("mock-sha-%d", p.shaSeq)
	p.branches[key(req.Owner, req.Repo, req.Branch)] = sha
	return ports.CommitResult{SHA: sha}, nil
}

func (p *MockProvider) CreatePullRequest(_ context.Context, req ports.PullRequestRequest) (ports.PullRequest, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := p.nextPR
	p.nextPR++
	pr := &ports.PullRequest{
		Number: n,
		URL:    fmt.Sprintf("https://github.com/%s/%s/pull/%d", req.Owner, req.Repo, n),
		State:  "open",
		Head:   req.Head,
		Base:   req.Base,
	}
	p.prs[fmt.Sprintf("%s/%s#%d", req.Owner, req.Repo, n)] = pr
	return *pr, nil
}

func (p *MockProvider) GetPullRequest(_ context.Context, owner, repo string, number int) (ports.PullRequest, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr, ok := p.prs[fmt.Sprintf("%s/%s#%d", owner, repo, number)]
	if !ok {
		return ports.PullRequest{}, shared.ErrNotFound
	}
	return *pr, nil
}

func (p *MockProvider) MergePullRequest(_ context.Context, owner, repo string, number int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr, ok := p.prs[fmt.Sprintf("%s/%s#%d", owner, repo, number)]
	if !ok {
		return shared.ErrNotFound
	}
	pr.State = "merged"
	return nil
}

func (p *MockProvider) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}
