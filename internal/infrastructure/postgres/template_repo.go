package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/template"
)

type TemplateRepository struct {
	pool *pgxpool.Pool
}

func NewTemplateRepository(pool *pgxpool.Pool) *TemplateRepository {
	return &TemplateRepository{pool: pool}
}

func (r *TemplateRepository) Create(ctx context.Context, t *template.ClusterTemplate) error {
	t.Touch()
	if t.ID == (shared.ID{}) {
		t.ID = shared.NewID()
	}
	specJSON, err := json.Marshal(t.Spec)
	if err != nil {
		return fmt.Errorf("marshaling template spec: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO cluster_templates (id, organization_id, name, version, description, provider_mode, spec, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		t.ID, t.OrganizationID, t.Name, t.Version, t.Description, t.ProviderMode, specJSON, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting cluster template: %w", err)
	}
	return nil
}

func (r *TemplateRepository) Get(ctx context.Context, id shared.ID) (*template.ClusterTemplate, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, organization_id, name, version, description, provider_mode, spec, created_at, updated_at
		 FROM cluster_templates WHERE id = $1`, id)
	return scanTemplate(row)
}

func (r *TemplateRepository) ListVersions(ctx context.Context, orgID shared.ID, name string) ([]*template.ClusterTemplate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, name, version, description, provider_mode, spec, created_at, updated_at
		 FROM cluster_templates WHERE organization_id = $1 AND name = $2 ORDER BY created_at DESC`, orgID, name)
	if err != nil {
		return nil, fmt.Errorf("listing template versions: %w", err)
	}
	defer rows.Close()
	return collectTemplates(rows)
}

func (r *TemplateRepository) List(ctx context.Context, orgID shared.ID, page shared.Page) ([]*template.ClusterTemplate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, name, version, description, provider_mode, spec, created_at, updated_at
		 FROM cluster_templates WHERE organization_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()
	return collectTemplates(rows)
}

func (r *TemplateRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM cluster_templates WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting cluster template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func collectTemplates(rows pgx.Rows) ([]*template.ClusterTemplate, error) {
	var out []*template.ClusterTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanTemplate(row rowScanner) (*template.ClusterTemplate, error) {
	t := &template.ClusterTemplate{}
	var providerMode string
	var specJSON []byte
	err := row.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Version, &t.Description, &providerMode, &specJSON, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning cluster template: %w", err)
	}
	t.ProviderMode = cluster.ProviderMode(providerMode)
	if err := json.Unmarshal(specJSON, &t.Spec); err != nil {
		return nil, fmt.Errorf("unmarshaling template spec: %w", err)
	}
	return t, nil
}
