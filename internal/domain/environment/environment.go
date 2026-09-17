// Package environment models an environment (e.g. development/staging/production)
// within a project. Clusters belong to exactly one environment.
package environment

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type Kind string

const (
	KindDevelopment Kind = "DEVELOPMENT"
	KindStaging     Kind = "STAGING"
	KindProduction  Kind = "PRODUCTION"
	KindCustom      Kind = "CUSTOM"
)

// Environment belongs to exactly one Project.
type Environment struct {
	ID        shared.ID
	ProjectID shared.ID
	Name      string
	Slug      string
	Kind      Kind
	shared.Timestamps
}

// Repository persists environments.
type Repository interface {
	Create(ctx context.Context, e *Environment) error
	Get(ctx context.Context, id shared.ID) (*Environment, error)
	ListByProject(ctx context.Context, projectID shared.ID, page shared.Page) ([]*Environment, error)
	Update(ctx context.Context, e *Environment) error
	Delete(ctx context.Context, id shared.ID) error
}
