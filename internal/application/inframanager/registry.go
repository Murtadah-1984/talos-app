// Package inframanager resolves the concrete ports.InfrastructureProvider
// implementation for a given infraprovider.Type (ADR-0007's "provider
// registry keyed on infraprovider.Type").
//
// LIMITATION (tracked in docs/roadmap.md, same pattern as the Talos/GitHub/
// Argo CD/Cluster API adapters): each provider type resolves to one
// process-wide instance built from global configuration, not a distinct
// instance per organization's own registered infraprovider.InfrastructureProvider
// row and credentials. Multi-tenant credential resolution is future work.
package inframanager

import (
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/infraprovider"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Registry resolves ports.InfrastructureProvider by infraprovider.Type.
type Registry struct {
	providers map[infraprovider.Type]ports.InfrastructureProvider
}

// NewRegistry builds a Registry from whichever providers are configured.
// Pass nil for a provider type that isn't wired up; For returns a clear
// error for it rather than a nil-pointer panic.
func NewRegistry(providers map[infraprovider.Type]ports.InfrastructureProvider) *Registry {
	return &Registry{providers: providers}
}

// For resolves the InfrastructureProvider implementation for t.
func (r *Registry) For(t infraprovider.Type) (ports.InfrastructureProvider, error) {
	p, ok := r.providers[t]
	if !ok || p == nil {
		return nil, fmt.Errorf("%w: no infrastructure provider implementation registered for type %s", shared.ErrNotImplemented, t)
	}
	return p, nil
}
