package ports

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// ApplicationStatus is the Argo CD Application status the platform observes
// for display and drift detection (ADR-0003). Argo CD remains authoritative;
// the platform never writes application/infrastructure state directly.
type ApplicationStatus struct {
	Name         string
	Namespace    string
	Project      string
	SyncStatus   gitops.ArgoCDSyncStatus
	HealthStatus gitops.ArgoCDHealthStatus
	Revision     string
	ResourceTree []ResourceNode
}

// ResourceNode is one node in an Argo CD Application's resource tree.
type ResourceNode struct {
	Kind      string
	Name      string
	Namespace string
	Health    gitops.ArgoCDHealthStatus
}

// SyncHistoryEntry is one prior sync operation for an Application.
type SyncHistoryEntry struct {
	Revision   string
	DeployedAt string
	Status     string
}

// ArgoCDClient observes and (on explicit request) triggers Argo CD
// synchronization. It never performs steady-state reconciliation itself.
type ArgoCDClient interface {
	ListApplications(ctx context.Context, project string) ([]ApplicationStatus, error)
	GetApplication(ctx context.Context, name string) (ApplicationStatus, error)
	Sync(ctx context.Context, name string) error
	GetHistory(ctx context.Context, name string) ([]SyncHistoryEntry, error)

	Capability() shared.CapabilityState
}
