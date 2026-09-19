package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// GitOpsRepository implements gitops.Repositories against Postgres.
type GitOpsRepository struct {
	pool *pgxpool.Pool
}

func NewGitOpsRepository(pool *pgxpool.Pool) *GitOpsRepository {
	return &GitOpsRepository{pool: pool}
}

func (r *GitOpsRepository) CreateGitProvider(ctx context.Context, g *gitops.GitProviderConfig) error {
	g.Touch()
	if g.ID == (shared.ID{}) {
		g.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO git_providers (id, organization_id, type, credential_ref, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		g.ID, g.OrganizationID, g.Type, nullableID(g.CredentialRef), g.CreatedAt, g.UpdatedAt)
	return err
}

func (r *GitOpsRepository) GetGitProvider(ctx context.Context, id shared.ID) (*gitops.GitProviderConfig, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, organization_id, type, credential_ref, created_at, updated_at FROM git_providers WHERE id = $1`, id)
	g := &gitops.GitProviderConfig{}
	var typ string
	var credRef *shared.ID
	err := row.Scan(&g.ID, &g.OrganizationID, &typ, &credRef, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning git provider: %w", err)
	}
	g.Type = gitops.GitProviderType(typ)
	if credRef != nil {
		g.CredentialRef = *credRef
	}
	return g, nil
}

func (r *GitOpsRepository) ListGitProviders(ctx context.Context, orgID shared.ID) ([]*gitops.GitProviderConfig, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, type, credential_ref, created_at, updated_at FROM git_providers WHERE organization_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*gitops.GitProviderConfig
	for rows.Next() {
		g := &gitops.GitProviderConfig{}
		var typ string
		var credRef *shared.ID
		if err := rows.Scan(&g.ID, &g.OrganizationID, &typ, &credRef, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		g.Type = gitops.GitProviderType(typ)
		if credRef != nil {
			g.CredentialRef = *credRef
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *GitOpsRepository) CreateRepository(ctx context.Context, repo *gitops.GitRepository) error {
	repo.Touch()
	if repo.ID == (shared.ID{}) {
		repo.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO git_repositories (id, organization_id, git_provider_id, owner, name, default_branch, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		repo.ID, repo.OrganizationID, repo.GitProviderID, repo.Owner, repo.Name, repo.DefaultBranch, repo.CreatedAt, repo.UpdatedAt)
	return err
}

func (r *GitOpsRepository) GetRepository(ctx context.Context, id shared.ID) (*gitops.GitRepository, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, organization_id, git_provider_id, owner, name, default_branch, created_at, updated_at
		 FROM git_repositories WHERE id = $1`, id)
	repo := &gitops.GitRepository{}
	err := row.Scan(&repo.ID, &repo.OrganizationID, &repo.GitProviderID, &repo.Owner, &repo.Name, &repo.DefaultBranch, &repo.CreatedAt, &repo.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning git repository: %w", err)
	}
	return repo, nil
}

func (r *GitOpsRepository) ListRepositories(ctx context.Context, orgID shared.ID) ([]*gitops.GitRepository, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, git_provider_id, owner, name, default_branch, created_at, updated_at
		 FROM git_repositories WHERE organization_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*gitops.GitRepository
	for rows.Next() {
		repo := &gitops.GitRepository{}
		if err := rows.Scan(&repo.ID, &repo.OrganizationID, &repo.GitProviderID, &repo.Owner, &repo.Name, &repo.DefaultBranch, &repo.CreatedAt, &repo.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, repo)
	}
	return out, rows.Err()
}

func (r *GitOpsRepository) CreateGitOpsConfiguration(ctx context.Context, c *gitops.GitOpsConfiguration) error {
	c.Touch()
	if c.ID == (shared.ID{}) {
		c.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO gitops_configurations (id, cluster_id, repository_id, path, branch, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.ClusterID, c.RepositoryID, c.Path, c.Branch, c.CreatedAt, c.UpdatedAt)
	return err
}

func (r *GitOpsRepository) GetGitOpsConfigurationForCluster(ctx context.Context, clusterID shared.ID) (*gitops.GitOpsConfiguration, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, cluster_id, repository_id, path, branch, created_at, updated_at FROM gitops_configurations WHERE cluster_id = $1`, clusterID)
	c := &gitops.GitOpsConfiguration{}
	err := row.Scan(&c.ID, &c.ClusterID, &c.RepositoryID, &c.Path, &c.Branch, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning gitops configuration: %w", err)
	}
	return c, nil
}

func (r *GitOpsRepository) CreateChangeSet(ctx context.Context, c *gitops.ChangeSet) error {
	c.Touch()
	if c.ID == (shared.ID{}) {
		c.ID = shared.NewID()
	}
	filesJSON, err := json.Marshal(c.GeneratedFiles)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO gitops_changesets (id, cluster_id, workflow_id, description, generated_files, commit_sha, pull_request_url, status, result, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		c.ID, c.ClusterID, c.WorkflowID, c.Description, filesJSON, c.CommitSHA, c.PullRequestURL, c.Status, c.Result, c.CreatedAt, c.UpdatedAt)
	return err
}

func (r *GitOpsRepository) UpdateChangeSet(ctx context.Context, c *gitops.ChangeSet) error {
	c.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE gitops_changesets SET commit_sha=$2, pull_request_url=$3, status=$4, result=$5, updated_at=$6 WHERE id=$1`,
		c.ID, c.CommitSHA, c.PullRequestURL, c.Status, c.Result, c.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *GitOpsRepository) GetChangeSet(ctx context.Context, id shared.ID) (*gitops.ChangeSet, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, cluster_id, workflow_id, description, generated_files, commit_sha, pull_request_url, status, result, created_at, updated_at
		 FROM gitops_changesets WHERE id = $1`, id)
	c := &gitops.ChangeSet{}
	var status string
	var filesJSON []byte
	err := row.Scan(&c.ID, &c.ClusterID, &c.WorkflowID, &c.Description, &filesJSON, &c.CommitSHA, &c.PullRequestURL, &status, &c.Result, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning change set: %w", err)
	}
	c.Status = gitops.ChangeSetStatus(status)
	_ = json.Unmarshal(filesJSON, &c.GeneratedFiles)
	return c, nil
}

func (r *GitOpsRepository) ListChangeSetsForCluster(ctx context.Context, clusterID shared.ID, page shared.Page) ([]*gitops.ChangeSet, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, cluster_id, workflow_id, description, generated_files, commit_sha, pull_request_url, status, result, created_at, updated_at
		 FROM gitops_changesets WHERE cluster_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		clusterID, page.Limit, page.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*gitops.ChangeSet
	for rows.Next() {
		c := &gitops.ChangeSet{}
		var status string
		var filesJSON []byte
		if err := rows.Scan(&c.ID, &c.ClusterID, &c.WorkflowID, &c.Description, &filesJSON, &c.CommitSHA, &c.PullRequestURL, &status, &c.Result, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Status = gitops.ChangeSetStatus(status)
		_ = json.Unmarshal(filesJSON, &c.GeneratedFiles)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *GitOpsRepository) UpsertArgoApplication(ctx context.Context, a *gitops.ArgoApplication) error {
	a.Touch()
	if a.ID == (shared.ID{}) {
		a.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO argo_applications (id, cluster_id, name, namespace, project, sync_status, health_status, revision, last_synced_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (cluster_id, name) DO UPDATE SET
			namespace = EXCLUDED.namespace, project = EXCLUDED.project, sync_status = EXCLUDED.sync_status,
			health_status = EXCLUDED.health_status, revision = EXCLUDED.revision, last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at`,
		a.ID, a.ClusterID, a.Name, a.Namespace, a.Project, a.SyncStatus, a.HealthStatus, a.Revision, a.LastSyncedAt, a.CreatedAt, a.UpdatedAt)
	return err
}

func (r *GitOpsRepository) ListArgoApplicationsForCluster(ctx context.Context, clusterID shared.ID) ([]*gitops.ArgoApplication, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, cluster_id, name, namespace, project, sync_status, health_status, revision, last_synced_at, created_at, updated_at
		 FROM argo_applications WHERE cluster_id = $1`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*gitops.ArgoApplication
	for rows.Next() {
		a := &gitops.ArgoApplication{}
		var sync, health string
		if err := rows.Scan(&a.ID, &a.ClusterID, &a.Name, &a.Namespace, &a.Project, &sync, &health, &a.Revision, &a.LastSyncedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.SyncStatus = gitops.ArgoCDSyncStatus(sync)
		a.HealthStatus = gitops.ArgoCDHealthStatus(health)
		out = append(out, a)
	}
	return out, rows.Err()
}
