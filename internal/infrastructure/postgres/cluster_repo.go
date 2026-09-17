package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type ClusterRepository struct {
	pool *pgxpool.Pool
}

func NewClusterRepository(pool *pgxpool.Pool) *ClusterRepository {
	return &ClusterRepository{pool: pool}
}

const clusterColumns = `id, organization_id, project_id, environment_id, site_id, template_id,
	name, endpoint, provider_mode, state, spec, git_commit_sha, created_at, updated_at`

func (r *ClusterRepository) Create(ctx context.Context, c *cluster.Cluster) error {
	c.Touch()
	if c.ID == (shared.ID{}) {
		c.ID = shared.NewID()
	}
	specJSON, err := json.Marshal(c.Spec)
	if err != nil {
		return fmt.Errorf("marshaling cluster spec: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO clusters (id, organization_id, project_id, environment_id, site_id, template_id,
			name, endpoint, provider_mode, state, spec, git_commit_sha, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		c.ID, c.OrganizationID, c.ProjectID, c.EnvironmentID, c.SiteID, c.TemplateID,
		c.Name, c.Endpoint, c.ProviderMode, c.State, specJSON, c.GitCommitSHA, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting cluster: %w", err)
	}
	return nil
}

func (r *ClusterRepository) Get(ctx context.Context, id shared.ID) (*cluster.Cluster, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+clusterColumns+` FROM clusters WHERE id = $1`, id)
	return scanCluster(row)
}

func (r *ClusterRepository) GetByName(ctx context.Context, orgID shared.ID, name string) (*cluster.Cluster, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+clusterColumns+` FROM clusters WHERE organization_id = $1 AND name = $2`, orgID, name)
	return scanCluster(row)
}

func (r *ClusterRepository) List(ctx context.Context, filter cluster.Filter, page shared.Page) ([]*cluster.Cluster, error) {
	where := []string{"1=1"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.OrganizationID != nil {
		where = append(where, "organization_id = "+arg(*filter.OrganizationID))
	}
	if filter.ProjectID != nil {
		where = append(where, "project_id = "+arg(*filter.ProjectID))
	}
	if filter.EnvironmentID != nil {
		where = append(where, "environment_id = "+arg(*filter.EnvironmentID))
	}
	if filter.SiteID != nil {
		where = append(where, "site_id = "+arg(*filter.SiteID))
	}
	if filter.ProviderMode != "" {
		where = append(where, "provider_mode = "+arg(filter.ProviderMode))
	}
	if filter.State != "" {
		where = append(where, "state = "+arg(filter.State))
	}
	if filter.KubernetesVersion != "" {
		where = append(where, "spec->>'KubernetesVersion' = "+arg(filter.KubernetesVersion))
	}
	if filter.TalosVersion != "" {
		where = append(where, "spec->>'TalosVersion' = "+arg(filter.TalosVersion))
	}

	limitArg, offsetArg := arg(page.Limit), arg(page.Offset)
	query := fmt.Sprintf(`SELECT %s FROM clusters WHERE %s ORDER BY created_at DESC LIMIT %s OFFSET %s`,
		clusterColumns, strings.Join(where, " AND "), limitArg, offsetArg)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing clusters: %w", err)
	}
	defer rows.Close()

	var out []*cluster.Cluster
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ClusterRepository) Update(ctx context.Context, c *cluster.Cluster) error {
	c.Touch()
	specJSON, err := json.Marshal(c.Spec)
	if err != nil {
		return fmt.Errorf("marshaling cluster spec: %w", err)
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE clusters SET name=$2, endpoint=$3, provider_mode=$4, state=$5, spec=$6, git_commit_sha=$7,
			template_id=$8, updated_at=$9 WHERE id=$1`,
		c.ID, c.Name, c.Endpoint, c.ProviderMode, c.State, specJSON, c.GitCommitSHA, c.TemplateID, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating cluster: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *ClusterRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting cluster: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func scanCluster(row rowScanner) (*cluster.Cluster, error) {
	c := &cluster.Cluster{}
	var specJSON []byte
	var providerMode, state string
	err := row.Scan(&c.ID, &c.OrganizationID, &c.ProjectID, &c.EnvironmentID, &c.SiteID, &c.TemplateID,
		&c.Name, &c.Endpoint, &providerMode, &state, &specJSON, &c.GitCommitSHA, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning cluster: %w", err)
	}
	c.ProviderMode = cluster.ProviderMode(providerMode)
	c.State = cluster.State(state)
	if err := json.Unmarshal(specJSON, &c.Spec); err != nil {
		return nil, fmt.Errorf("unmarshaling cluster spec: %w", err)
	}
	return c, nil
}
