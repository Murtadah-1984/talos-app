package ports

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// FileChange is one file to write (or delete, when Delete is true) in a commit.
type FileChange struct {
	Path    string
	Content []byte
	Delete  bool
}

// CommitRequest describes a commit to make on a branch.
type CommitRequest struct {
	Owner   string
	Repo    string
	Branch  string
	Message string
	Files   []FileChange
}

// CommitResult is the outcome of a commit.
type CommitResult struct {
	SHA string
}

// PullRequestRequest describes a PR to open.
type PullRequestRequest struct {
	Owner string
	Repo  string
	Title string
	Body  string
	Head  string
	Base  string
}

// PullRequest mirrors the subset of GitHub PR fields the platform cares about.
type PullRequest struct {
	Number int
	URL    string
	State  string // "open", "closed", "merged"
	Head   string
	Base   string
}

// GitProvider is the platform's only path to Git hosting (GitHub initially).
// The platform generates commits/PRs; it never mutates production state
// directly (ADR-0004, §19).
type GitProvider interface {
	CreateBranch(ctx context.Context, owner, repo, branch, fromRef string) error
	GetFile(ctx context.Context, owner, repo, ref, path string) ([]byte, error)
	Commit(ctx context.Context, req CommitRequest) (CommitResult, error)
	CreatePullRequest(ctx context.Context, req PullRequestRequest) (PullRequest, error)
	GetPullRequest(ctx context.Context, owner, repo string, number int) (PullRequest, error)
	MergePullRequest(ctx context.Context, owner, repo string, number int) error
	// GetCommitParent returns the SHA of sha's first parent commit, needed
	// to resolve the tree a change replaced when rolling it back via Git
	// revert (§18, §20). Returns shared.ErrNotFound if sha has no parent
	// (the repository's very first commit).
	GetCommitParent(ctx context.Context, owner, repo, sha string) (string, error)

	Capability() shared.CapabilityState
}
