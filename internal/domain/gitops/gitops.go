// Package gitops models Git providers/repositories and the Argo CD/Cluster
// API resources the platform observes there (§19, §20, ADR-0003, ADR-0004).
package gitops

import (
	"context"
	"time"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// GitProviderType identifies which GitProvider implementation to use.
type GitProviderType string

const (
	GitProviderGitHub GitProviderType = "GITHUB"
)

// GitProviderConfig is a registered Git hosting account/app installation.
type GitProviderConfig struct {
	ID             shared.ID
	OrganizationID shared.ID
	Type           GitProviderType
	CredentialRef  shared.ID
	shared.Timestamps
}

// GitRepository is a Git repository the platform reads/writes GitOps state to.
type GitRepository struct {
	ID             shared.ID
	OrganizationID shared.ID
	GitProviderID  shared.ID
	Owner          string
	Name           string
	DefaultBranch  string
	shared.Timestamps
}

// GitOpsConfiguration binds a cluster (or infra/app scope) to a path within a
// GitRepository, per the layout in §3.
type GitOpsConfiguration struct {
	ID           shared.ID
	ClusterID    shared.ID
	RepositoryID shared.ID
	Path         string
	Branch       string
	shared.Timestamps
}

// ChangeSetStatus is the lifecycle of a single GitOps change (§20).
type ChangeSetStatus string

const (
	ChangeSetPending   ChangeSetStatus = "PENDING"
	ChangeSetValidated ChangeSetStatus = "VALIDATED"
	ChangeSetCommitted ChangeSetStatus = "COMMITTED"
	ChangeSetPRCreated ChangeSetStatus = "PR_CREATED"
	ChangeSetMerged    ChangeSetStatus = "MERGED"
	ChangeSetSynced    ChangeSetStatus = "SYNCED"
	ChangeSetFailed    ChangeSetStatus = "FAILED"
)

// ChangeSet records the lifecycle of one meaningful infrastructure change,
// from generated files through PR merge and Argo CD sync (§20).
type ChangeSet struct {
	ID             shared.ID
	ClusterID      shared.ID
	WorkflowID     *shared.ID
	Description    string
	GeneratedFiles []string
	PullRequestURL string
	Status         ChangeSetStatus
	Result         string
	shared.Timestamps
}

// ArgoCDSyncStatus mirrors Argo CD's own Application sync status vocabulary.
type ArgoCDSyncStatus string

const (
	SyncStatusUnknown   ArgoCDSyncStatus = "Unknown"
	SyncStatusSynced    ArgoCDSyncStatus = "Synced"
	SyncStatusOutOfSync ArgoCDSyncStatus = "OutOfSync"
)

// ArgoCDHealthStatus mirrors Argo CD's own Application health vocabulary.
type ArgoCDHealthStatus string

const (
	HealthUnknown     ArgoCDHealthStatus = "Unknown"
	HealthProgressing ArgoCDHealthStatus = "Progressing"
	HealthHealthy     ArgoCDHealthStatus = "Healthy"
	HealthSuspended   ArgoCDHealthStatus = "Suspended"
	HealthDegraded    ArgoCDHealthStatus = "Degraded"
	HealthMissing     ArgoCDHealthStatus = "Missing"
)

// ArgoApplication is an observed snapshot of an Argo CD Application, cached
// for display; Argo CD itself remains authoritative (ADR-0003).
type ArgoApplication struct {
	ID           shared.ID
	ClusterID    shared.ID
	Name         string
	Namespace    string
	Project      string
	SyncStatus   ArgoCDSyncStatus
	HealthStatus ArgoCDHealthStatus
	Revision     string
	LastSyncedAt *time.Time
	shared.Timestamps
}

// Repositories bundles the persistence ports for this package's aggregates.
type Repositories interface {
	CreateGitProvider(ctx context.Context, g *GitProviderConfig) error
	GetGitProvider(ctx context.Context, id shared.ID) (*GitProviderConfig, error)
	ListGitProviders(ctx context.Context, orgID shared.ID) ([]*GitProviderConfig, error)

	CreateRepository(ctx context.Context, r *GitRepository) error
	GetRepository(ctx context.Context, id shared.ID) (*GitRepository, error)
	ListRepositories(ctx context.Context, orgID shared.ID) ([]*GitRepository, error)

	CreateGitOpsConfiguration(ctx context.Context, c *GitOpsConfiguration) error
	GetGitOpsConfigurationForCluster(ctx context.Context, clusterID shared.ID) (*GitOpsConfiguration, error)

	CreateChangeSet(ctx context.Context, c *ChangeSet) error
	UpdateChangeSet(ctx context.Context, c *ChangeSet) error
	ListChangeSetsForCluster(ctx context.Context, clusterID shared.ID, page shared.Page) ([]*ChangeSet, error)

	UpsertArgoApplication(ctx context.Context, a *ArgoApplication) error
	ListArgoApplicationsForCluster(ctx context.Context, clusterID shared.ID) ([]*ArgoApplication, error)
}
