// Package template models versioned, reusable cluster templates (§13).
package template

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// ClusterTemplate is a versioned, reusable cluster configuration.
type ClusterTemplate struct {
	ID             shared.ID
	OrganizationID shared.ID
	Name           string
	Version        string
	Description    string
	ProviderMode   cluster.ProviderMode
	Spec           cluster.Spec
	shared.Timestamps
}

// Repository persists cluster templates. Multiple versions of the same
// template name may exist; templates are immutable once created (a new
// version is a new row) so clusters can pin a specific version.
type Repository interface {
	Create(ctx context.Context, t *ClusterTemplate) error
	Get(ctx context.Context, id shared.ID) (*ClusterTemplate, error)
	ListVersions(ctx context.Context, orgID shared.ID, name string) ([]*ClusterTemplate, error)
	List(ctx context.Context, orgID shared.ID, page shared.Page) ([]*ClusterTemplate, error)
	Delete(ctx context.Context, id shared.ID) error
}
