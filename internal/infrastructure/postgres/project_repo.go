package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/project"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type ProjectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

func (r *ProjectRepository) Create(ctx context.Context, p *project.Project) error {
	p.Touch()
	if p.ID == (shared.ID{}) {
		p.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO projects (id, organization_id, name, slug, description, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.ID, p.OrganizationID, p.Name, p.Slug, p.Description, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting project: %w", err)
	}
	return nil
}

func (r *ProjectRepository) Get(ctx context.Context, id shared.ID) (*project.Project, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, organization_id, name, slug, description, created_at, updated_at FROM projects WHERE id = $1`, id)
	return scanProject(row)
}

func (r *ProjectRepository) ListByOrganization(ctx context.Context, orgID shared.ID, page shared.Page) ([]*project.Project, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, name, slug, description, created_at, updated_at
		 FROM projects WHERE organization_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	defer rows.Close()

	var out []*project.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepository) Update(ctx context.Context, p *project.Project) error {
	p.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE projects SET name = $2, slug = $3, description = $4, updated_at = $5 WHERE id = $1`,
		p.ID, p.Name, p.Slug, p.Description, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *ProjectRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func scanProject(row rowScanner) (*project.Project, error) {
	p := &project.Project{}
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning project: %w", err)
	}
	return p, nil
}
