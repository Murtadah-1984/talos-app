package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/infraprovider"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type InfraProviderRepository struct {
	pool *pgxpool.Pool
}

func NewInfraProviderRepository(pool *pgxpool.Pool) *InfraProviderRepository {
	return &InfraProviderRepository{pool: pool}
}

func (r *InfraProviderRepository) Create(ctx context.Context, p *infraprovider.InfrastructureProvider) error {
	p.Touch()
	if p.ID == (shared.ID{}) {
		p.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO infrastructure_providers (id, organization_id, name, type, endpoint, credential_ref, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ID, p.OrganizationID, p.Name, p.Type, p.Endpoint, nullableID(p.CredentialRef), p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting infrastructure provider: %w", err)
	}
	return nil
}

func (r *InfraProviderRepository) Get(ctx context.Context, id shared.ID) (*infraprovider.InfrastructureProvider, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, organization_id, name, type, endpoint, credential_ref, created_at, updated_at
		 FROM infrastructure_providers WHERE id = $1`, id)
	return scanInfraProvider(row)
}

func (r *InfraProviderRepository) ListByOrganization(ctx context.Context, orgID shared.ID, page shared.Page) ([]*infraprovider.InfrastructureProvider, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, name, type, endpoint, credential_ref, created_at, updated_at
		 FROM infrastructure_providers WHERE organization_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing infrastructure providers: %w", err)
	}
	defer rows.Close()

	var out []*infraprovider.InfrastructureProvider
	for rows.Next() {
		p, err := scanInfraProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *InfraProviderRepository) Update(ctx context.Context, p *infraprovider.InfrastructureProvider) error {
	p.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE infrastructure_providers SET name=$2, type=$3, endpoint=$4, credential_ref=$5, updated_at=$6 WHERE id=$1`,
		p.ID, p.Name, p.Type, p.Endpoint, nullableID(p.CredentialRef), p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating infrastructure provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *InfraProviderRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM infrastructure_providers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting infrastructure provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *InfraProviderRepository) PutCredential(ctx context.Context, c *infraprovider.Credential) error {
	c.Touch()
	if c.ID == (shared.ID{}) {
		c.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO provider_credentials (id, owner_id, ciphertext, key_ref, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		c.ID, c.OwnerID, c.Ciphertext, c.KeyRef, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting provider credential: %w", err)
	}
	return nil
}

func (r *InfraProviderRepository) GetCredential(ctx context.Context, id shared.ID) (*infraprovider.Credential, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, owner_id, ciphertext, key_ref, created_at, updated_at FROM provider_credentials WHERE id = $1`, id)
	c := &infraprovider.Credential{}
	err := row.Scan(&c.ID, &c.OwnerID, &c.Ciphertext, &c.KeyRef, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning provider credential: %w", err)
	}
	return c, nil
}

func (r *InfraProviderRepository) DeleteCredential(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM provider_credentials WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting provider credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func scanInfraProvider(row rowScanner) (*infraprovider.InfrastructureProvider, error) {
	p := &infraprovider.InfrastructureProvider{}
	var typ string
	var credRef *shared.ID
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &typ, &p.Endpoint, &credRef, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning infrastructure provider: %w", err)
	}
	p.Type = infraprovider.Type(typ)
	if credRef != nil {
		p.CredentialRef = *credRef
	}
	return p, nil
}
