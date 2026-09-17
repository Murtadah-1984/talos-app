// Package organization models the top level of the tenancy hierarchy:
// Organization -> Project -> Environment -> Cluster.
package organization

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Organization is the top-level tenant boundary in the platform.
type Organization struct {
	ID   shared.ID
	Name string
	Slug string
	shared.Timestamps
}

// Repository persists organizations. Implemented by internal/infrastructure/postgres.
type Repository interface {
	Create(ctx context.Context, org *Organization) error
	Get(ctx context.Context, id shared.ID) (*Organization, error)
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	List(ctx context.Context, page shared.Page) ([]*Organization, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id shared.ID) error
}
