package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/environment"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type EnvironmentRepository struct {
	pool *pgxpool.Pool
}

func NewEnvironmentRepository(pool *pgxpool.Pool) *EnvironmentRepository {
	return &EnvironmentRepository{pool: pool}
}

func (r *EnvironmentRepository) Create(ctx context.Context, e *environment.Environment) error {
	e.Touch()
	if e.ID == (shared.ID{}) {
		e.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO environments (id, project_id, name, slug, kind, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.ID, e.ProjectID, e.Name, e.Slug, e.Kind, e.CreatedAt, e.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting environment: %w", err)
	}
	return nil
}

func (r *EnvironmentRepository) Get(ctx context.Context, id shared.ID) (*environment.Environment, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, project_id, name, slug, kind, created_at, updated_at FROM environments WHERE id = $1`, id)
	return scanEnvironment(row)
}

func (r *EnvironmentRepository) ListByProject(ctx context.Context, projectID shared.ID, page shared.Page) ([]*environment.Environment, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, name, slug, kind, created_at, updated_at
		 FROM environments WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		projectID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}
	defer rows.Close()

	var out []*environment.Environment
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *EnvironmentRepository) Update(ctx context.Context, e *environment.Environment) error {
	e.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE environments SET name = $2, slug = $3, kind = $4, updated_at = $5 WHERE id = $1`,
		e.ID, e.Name, e.Slug, e.Kind, e.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating environment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *EnvironmentRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM environments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting environment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func scanEnvironment(row rowScanner) (*environment.Environment, error) {
	e := &environment.Environment{}
	var kind string
	err := row.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Slug, &kind, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning environment: %w", err)
	}
	e.Kind = environment.Kind(kind)
	return e, nil
}
