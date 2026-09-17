package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/organization"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// OrganizationRepository implements organization.Repository against Postgres.
type OrganizationRepository struct {
	pool *pgxpool.Pool
}

func NewOrganizationRepository(pool *pgxpool.Pool) *OrganizationRepository {
	return &OrganizationRepository{pool: pool}
}

func (r *OrganizationRepository) Create(ctx context.Context, o *organization.Organization) error {
	o.Touch()
	if o.ID == (shared.ID{}) {
		o.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO organizations (id, name, slug, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)`,
		o.ID, o.Name, o.Slug, o.CreatedAt, o.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting organization: %w", err)
	}
	return nil
}

func (r *OrganizationRepository) Get(ctx context.Context, id shared.ID) (*organization.Organization, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, name, slug, created_at, updated_at FROM organizations WHERE id = $1`, id)
	return scanOrganization(row)
}

func (r *OrganizationRepository) GetBySlug(ctx context.Context, slug string) (*organization.Organization, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, name, slug, created_at, updated_at FROM organizations WHERE slug = $1`, slug)
	return scanOrganization(row)
}

func (r *OrganizationRepository) List(ctx context.Context, page shared.Page) ([]*organization.Organization, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, slug, created_at, updated_at FROM organizations ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}
	defer rows.Close()

	var out []*organization.Organization
	for rows.Next() {
		o, err := scanOrganization(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *OrganizationRepository) Update(ctx context.Context, o *organization.Organization) error {
	o.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE organizations SET name = $2, slug = $3, updated_at = $4 WHERE id = $1`,
		o.ID, o.Name, o.Slug, o.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating organization: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *OrganizationRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting organization: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrganization(row rowScanner) (*organization.Organization, error) {
	o := &organization.Organization{}
	err := row.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning organization: %w", err)
	}
	return o, nil
}
