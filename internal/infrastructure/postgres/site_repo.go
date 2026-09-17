package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/site"
)

type SiteRepository struct {
	pool *pgxpool.Pool
}

func NewSiteRepository(pool *pgxpool.Pool) *SiteRepository {
	return &SiteRepository{pool: pool}
}

func (r *SiteRepository) Create(ctx context.Context, s *site.Site) error {
	s.Touch()
	if s.ID == (shared.ID{}) {
		s.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sites (id, organization_id, name, country, city, network, infrastructure_provider, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		s.ID, s.OrganizationID, s.Name, s.Country, s.City, s.Network, nullableID(s.InfrastructureProvider), s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting site: %w", err)
	}
	return nil
}

func (r *SiteRepository) Get(ctx context.Context, id shared.ID) (*site.Site, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, organization_id, name, country, city, network, infrastructure_provider, created_at, updated_at
		 FROM sites WHERE id = $1`, id)
	return scanSite(row)
}

func (r *SiteRepository) ListByOrganization(ctx context.Context, orgID shared.ID, page shared.Page) ([]*site.Site, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, name, country, city, network, infrastructure_provider, created_at, updated_at
		 FROM sites WHERE organization_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing sites: %w", err)
	}
	defer rows.Close()

	var out []*site.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SiteRepository) Update(ctx context.Context, s *site.Site) error {
	s.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE sites SET name = $2, country = $3, city = $4, network = $5, infrastructure_provider = $6, updated_at = $7 WHERE id = $1`,
		s.ID, s.Name, s.Country, s.City, s.Network, nullableID(s.InfrastructureProvider), s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating site: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *SiteRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM sites WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting site: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func scanSite(row rowScanner) (*site.Site, error) {
	s := &site.Site{}
	var provider *shared.ID
	err := row.Scan(&s.ID, &s.OrganizationID, &s.Name, &s.Country, &s.City, &s.Network, &provider, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning site: %w", err)
	}
	if provider != nil {
		s.InfrastructureProvider = *provider
	}
	return s, nil
}

// nullableID returns nil when id is the zero value, so optional foreign keys
// are written as SQL NULL instead of an all-zero UUID.
func nullableID(id shared.ID) any {
	if id == (shared.ID{}) {
		return nil
	}
	return id
}
