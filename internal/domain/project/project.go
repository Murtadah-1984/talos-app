// Package project models a project: a grouping of environments within an organization.
package project

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Project belongs to exactly one Organization.
type Project struct {
	ID             shared.ID
	OrganizationID shared.ID
	Name           string
	Slug           string
	Description    string
	shared.Timestamps
}

// Repository persists projects.
type Repository interface {
	Create(ctx context.Context, p *Project) error
	Get(ctx context.Context, id shared.ID) (*Project, error)
	ListByOrganization(ctx context.Context, orgID shared.ID, page shared.Page) ([]*Project, error)
	Update(ctx context.Context, p *Project) error
	Delete(ctx context.Context, id shared.ID) error
}
