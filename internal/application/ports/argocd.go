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

// ApplicationSetCondition mirrors one entry of an Argo CD ApplicationSet's
// status.conditions (e.g. "ResourcesUpToDate", "ErrorOccurred").
type ApplicationSetCondition struct {
	Type    string
	Status  string
	Message string
}

// ApplicationSetStatus is the Argo CD ApplicationSet status the platform
// observes: which Applications it generated and its own health/error
// conditions (ADR-0003). Unlike ApplicationStatus, this is a live,
// unpersisted read — the platform doesn't yet cache ApplicationSets the way
// it caches individual Applications (see docs/argocd/README.md).
type ApplicationSetStatus struct {
	Name       string
	Namespace  string
	Resources  []string // names of Applications this ApplicationSet generated
	Conditions []ApplicationSetCondition
}

// ArgoCDClient observes and (on explicit request) triggers Argo CD
// synchronization. It never performs steady-state reconciliation itself.
type ArgoCDClient interface {
	ListApplications(ctx context.Context, project string) ([]ApplicationStatus, error)
	GetApplication(ctx context.Context, name string) (ApplicationStatus, error)
	Sync(ctx context.Context, name string) error
	GetHistory(ctx context.Context, name string) ([]SyncHistoryEntry, error)
	ListApplicationSets(ctx context.Context, project string) ([]ApplicationSetStatus, error)

	Capability() shared.CapabilityState
}
