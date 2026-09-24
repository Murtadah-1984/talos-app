// Package workflow models the durable, resumable workflow engine described in
// ADR-0005: a Workflow is a named sequence of persisted Steps executed by
// platform-worker processes.
package workflow

import (
	"context"
	"time"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Status is the lifecycle state of a workflow run or step.
type Status string

const (
	StatusPending        Status = "PENDING"
	StatusRunning        Status = "RUNNING"
	StatusSucceeded      Status = "SUCCEEDED"
	StatusFailed         Status = "FAILED"
	StatusNeedsAttention Status = "NEEDS_ATTENTION"
	StatusCancelled      Status = "CANCELLED"
	// StatusAwaitingApproval is a non-error pause (§4 human-in-the-loop
	// approval gate): a step reported there is nothing more to do until an
	// external event arrives (typically a GitHub PR-merged webhook), so the
	// engine stops dispatching this workflow until something calls
	// Engine.Resume — unlike NEEDS_ATTENTION, this isn't a failure an
	// operator needs to fix, just a wait.
	StatusAwaitingApproval Status = "AWAITING_APPROVAL"
)

// Type identifies which registered step sequence a workflow runs (§23).
type Type string

const (
	TypeClusterProvision Type = "CLUSTER_PROVISION"
	TypeClusterUpgrade   Type = "CLUSTER_UPGRADE"
	TypeMachineUpgrade   Type = "MACHINE_UPGRADE"
	TypeMachineDiscovery Type = "MACHINE_DISCOVERY"
	TypeClusterDelete    Type = "CLUSTER_DELETE"
	TypeWorkerScale      Type = "WORKER_SCALE"
)

// Workflow is one durable run of a named workflow Type against a target
// resource (typically a cluster). Every run carries an idempotency key so
// re-submitting the same request never creates a duplicate run (§34).
type Workflow struct {
	ID             shared.ID
	Type           Type
	IdempotencyKey string
	ClusterID      *shared.ID
	MachineID      *shared.ID
	RequestedBy    shared.ID
	Input          map[string]any
	Status         Status
	CurrentStep    int
	Error          string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	shared.Timestamps
}

// Step is one persisted unit of work within a Workflow.
type Step struct {
	ID         shared.ID
	WorkflowID shared.ID
	Sequence   int
	Name       string
	Status     Status
	Output     map[string]any
	Error      string
	Attempts   int
	StartedAt  *time.Time
	FinishedAt *time.Time
	shared.Timestamps
}

// Repository persists workflows and steps, and provides the claim primitive
// platform-worker uses to safely pick up pending work across replicas.
type Repository interface {
	Create(ctx context.Context, w *Workflow, steps []*Step) error
	Get(ctx context.Context, id shared.ID) (*Workflow, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Workflow, error)
	List(ctx context.Context, filter Filter, page shared.Page) ([]*Workflow, error)
	Update(ctx context.Context, w *Workflow) error

	ListSteps(ctx context.Context, workflowID shared.ID) ([]*Step, error)
	UpdateStep(ctx context.Context, s *Step) error

	// ClaimNextStep locks and returns the next pending step for workflowID
	// using SELECT ... FOR UPDATE SKIP LOCKED semantics, so two workers never
	// execute the same step concurrently (ADR-0005).
	ClaimNextStep(ctx context.Context, workflowID shared.ID) (*Step, error)
}

// Filter narrows a workflow list query.
type Filter struct {
	ClusterID *shared.ID
	Type      Type
	Status    Status
}
