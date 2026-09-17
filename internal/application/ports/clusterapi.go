package ports

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// CAPIResourceStatus is a read-only observation of a Cluster API resource in
// the management cluster (ADR-0002). The platform generates manifests and
// commits them to Git; it never reconciles CAPI resources itself.
type CAPIResourceStatus struct {
	Kind      string // "Cluster", "MachineDeployment", "Machine", ...
	Name      string
	Namespace string
	Phase     string
	Ready     bool
}

// ClusterAPIProvider renders CAPI manifests for a cluster.Spec and observes
// the resulting resources' status in the management cluster.
type ClusterAPIProvider interface {
	RenderManifests(ctx context.Context, c *cluster.Cluster) ([]FileChange, error)
	GetClusterStatus(ctx context.Context, namespace, name string) ([]CAPIResourceStatus, error)

	Capability() shared.CapabilityState
}
