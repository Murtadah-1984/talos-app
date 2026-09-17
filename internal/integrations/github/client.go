// Package github implements the ports.GitProvider adapter for GitHub
// (§19, ADR-0004). This file is the Phase 3 real implementation, backed by
// the official go-github client and the Git Data API (so a single commit can
// atomically add/update/delete several files, matching how the platform
// commits a whole manifest set at once). See mock.go for the deterministic
// stand-in used elsewhere in local development.
package github

import (
	"context"
	"fmt"

	ghapi "github.com/google/go-github/v75/github"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Client is the real ports.GitProvider adapter, authenticated with a single
// personal access token or GitHub App installation token.
type Client struct {
	gh *ghapi.Client
}

// NewClient builds a Client authenticated with token (a PAT or installation
// token). The token never leaves this package's calls to the GitHub API.
func NewClient(token string) *Client {
	return &Client{gh: ghapi.NewClient(nil).WithAuthToken(token)}
}

// LoadClientFromSecretStore resolves a GitHub token from the platform's
// SecretStore (ADR-0006) and builds a Client from it, mirroring
// talos.LoadClientFromSecretStore.
func LoadClientFromSecretStore(ctx context.Context, store ports.SecretStore, ref ports.SecretRef) (*Client, error) {
	data, err := store.Get(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("loading GitHub token from secret store: %w", err)
	}
	return NewClient(string(data)), nil
}

func refName(branch string) string { return "refs/heads/" + branch }

func (c *Client) CreateBranch(ctx context.Context, owner, repo, branch, fromRef string) error {
	base, _, err := c.gh.Git.GetRef(ctx, owner, repo, refName(fromRef))
	if err != nil {
		return fmt.Errorf("resolving base ref %s: %w", fromRef, err)
	}
	_, _, err = c.gh.Git.CreateRef(ctx, owner, repo, ghapi.CreateRef{
		Ref: refName(branch),
		SHA: base.GetObject().GetSHA(),
	})
	if err != nil {
		return fmt.Errorf("creating branch %s: %w", branch, err)
	}
	return nil
}

func (c *Client) GetFile(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	file, _, _, err := c.gh.Repositories.GetContents(ctx, owner, repo, path, &ghapi.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return nil, fmt.Errorf("getting %s@%s:%s: %w", owner, repo, path, err)
	}
	if file == nil {
		return nil, fmt.Errorf("%w: %s:%s is a directory, not a file", shared.ErrInvalidInput, path, ref)
	}
	content, err := file.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decoding file content: %w", err)
	}
	return []byte(content), nil
}

// Commit creates one atomic commit covering every file in req.Files via the
// Git Data API (blob content is inlined directly on each tree entry, so no
// separate CreateBlob round trip is needed for text files), then fast-forwards
// req.Branch to it.
func (c *Client) Commit(ctx context.Context, req ports.CommitRequest) (ports.CommitResult, error) {
	branchRef, _, err := c.gh.Git.GetRef(ctx, req.Owner, req.Repo, refName(req.Branch))
	if err != nil {
		return ports.CommitResult{}, fmt.Errorf("resolving branch %s: %w", req.Branch, err)
	}
	baseSHA := branchRef.GetObject().GetSHA()

	baseCommit, _, err := c.gh.Git.GetCommit(ctx, req.Owner, req.Repo, baseSHA)
	if err != nil {
		return ports.CommitResult{}, fmt.Errorf("getting base commit %s: %w", baseSHA, err)
	}

	entries := make([]*ghapi.TreeEntry, 0, len(req.Files))
	for _, f := range req.Files {
		entry := &ghapi.TreeEntry{
			Path: ghapi.Ptr(f.Path),
			Mode: ghapi.Ptr("100644"),
		}
		if !f.Delete {
			entry.Type = ghapi.Ptr("blob")
			entry.Content = ghapi.Ptr(string(f.Content))
		}
		// A deleted entry is left with SHA and Content both nil, which the
		// GitHub API interprets as "remove this path from the tree".
		entries = append(entries, entry)
	}

	tree, _, err := c.gh.Git.CreateTree(ctx, req.Owner, req.Repo, baseCommit.GetTree().GetSHA(), entries)
	if err != nil {
		return ports.CommitResult{}, fmt.Errorf("creating tree: %w", err)
	}

	commit, _, err := c.gh.Git.CreateCommit(ctx, req.Owner, req.Repo, ghapi.Commit{
		Message: ghapi.Ptr(req.Message),
		Tree:    tree,
		Parents: []*ghapi.Commit{{SHA: ghapi.Ptr(baseSHA)}},
	}, nil)
	if err != nil {
		return ports.CommitResult{}, fmt.Errorf("creating commit: %w", err)
	}

	if _, _, err := c.gh.Git.UpdateRef(ctx, req.Owner, req.Repo, refName(req.Branch), ghapi.UpdateRef{SHA: commit.GetSHA()}); err != nil {
		return ports.CommitResult{}, fmt.Errorf("updating branch %s: %w", req.Branch, err)
	}

	return ports.CommitResult{SHA: commit.GetSHA()}, nil
}

func (c *Client) CreatePullRequest(ctx context.Context, req ports.PullRequestRequest) (ports.PullRequest, error) {
	pr, _, err := c.gh.PullRequests.Create(ctx, req.Owner, req.Repo, &ghapi.NewPullRequest{
		Title: ghapi.Ptr(req.Title),
		Body:  ghapi.Ptr(req.Body),
		Head:  ghapi.Ptr(req.Head),
		Base:  ghapi.Ptr(req.Base),
	})
	if err != nil {
		return ports.PullRequest{}, fmt.Errorf("creating pull request: %w", err)
	}
	return toPullRequest(pr), nil
}

func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (ports.PullRequest, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return ports.PullRequest{}, fmt.Errorf("getting pull request #%d: %w", number, err)
	}
	return toPullRequest(pr), nil
}

func (c *Client) MergePullRequest(ctx context.Context, owner, repo string, number int) error {
	result, _, err := c.gh.PullRequests.Merge(ctx, owner, repo, number, "", nil)
	if err != nil {
		return fmt.Errorf("merging pull request #%d: %w", number, err)
	}
	if !result.GetMerged() {
		return fmt.Errorf("pull request #%d was not merged: %s", number, result.GetMessage())
	}
	return nil
}

func toPullRequest(pr *ghapi.PullRequest) ports.PullRequest {
	state := pr.GetState()
	if pr.GetMerged() {
		state = "merged"
	}
	return ports.PullRequest{
		Number: pr.GetNumber(),
		URL:    pr.GetHTMLURL(),
		State:  state,
		Head:   pr.GetHead().GetRef(),
		Base:   pr.GetBase().GetRef(),
	}
}

func (c *Client) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}

var _ ports.GitProvider = (*Client)(nil)
