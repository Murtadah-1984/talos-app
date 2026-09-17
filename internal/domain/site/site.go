// Package site models a physical or logical location that hosts infrastructure
// (a datacenter, a colo cage, a cloud region). Sites are configuration, never hardcoded.
package site

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Site is a physical or logical location associated with one infrastructure provider.
type Site struct {
	ID                     shared.ID
	OrganizationID         shared.ID
	Name                   string
	Country                string
	City                   string
	Network                string
	InfrastructureProvider shared.ID // references infraprovider.InfrastructureProvider
	shared.Timestamps
}

// Repository persists sites.
type Repository interface {
	Create(ctx context.Context, s *Site) error
	Get(ctx context.Context, id shared.ID) (*Site, error)
	ListByOrganization(ctx context.Context, orgID shared.ID, page shared.Page) ([]*Site, error)
	Update(ctx context.Context, s *Site) error
	Delete(ctx context.Context, id shared.ID) error
}
