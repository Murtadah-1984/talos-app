// Package operation models imperative, point-in-time actions (reboot, drain,
// cordon, upgrade-single-machine) as distinct from the multi-step declarative
// Workflow aggregate. An Operation may be dispatched standalone from the API,
// or as the concrete action a workflow.Step performs.
package operation

import (
	"context"
	"time"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Kind enumerates supported imperative operations (§17).
type Kind string

const (
	KindRebootMachine     Kind = "REBOOT_MACHINE"
	KindShutdownMachine   Kind = "SHUTDOWN_MACHINE"
	KindDrainNode         Kind = "DRAIN_NODE"
	KindCordonNode        Kind = "CORDON_NODE"
	KindUncordonNode      Kind = "UNCORDON_NODE"
	KindUpgradeTalos      Kind = "UPGRADE_TALOS"
	KindUpgradeKubernetes Kind = "UPGRADE_KUBERNETES"
	KindResetMachine      Kind = "RESET_MACHINE"
	KindRemoveEtcdMember  Kind = "REMOVE_ETCD_MEMBER"
	KindDestroyCluster    Kind = "DESTROY_CLUSTER"

	// Hard power operations go through the infrastructure provider's BMC/
	// hypervisor API (ADR-0007, §11/§12) rather than Talos — they work even
	// when the OS itself is unresponsive, unlike KindRebootMachine/
	// KindShutdownMachine which ask Talos to do it gracefully.
	KindHardPowerOn    Kind = "HARD_POWER_ON"
	KindHardPowerOff   Kind = "HARD_POWER_OFF"
	KindHardPowerCycle Kind = "HARD_POWER_CYCLE"
)

// Danger reports whether a Kind requires explicit confirmation + elevated RBAC (§17).
func (k Kind) Danger() bool {
	switch k {
	case KindResetMachine, KindRemoveEtcdMember, KindDestroyCluster, KindUpgradeTalos,
		KindHardPowerOff, KindHardPowerCycle:
		return true
	default:
		return false
	}
}

// Status is the lifecycle of an operation.
type Status string

const (
	StatusRequested Status = "REQUESTED"
	StatusRunning   Status = "RUNNING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
)

// Operation is one imperative action requested against a machine or cluster.
type Operation struct {
	ID             shared.ID
	IdempotencyKey string
	Kind           Kind
	ClusterID      *shared.ID
	MachineID      *shared.ID
	RequestedBy    shared.ID
	Status         Status
	Result         string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	shared.Timestamps
}

// Repository persists operations.
type Repository interface {
	Create(ctx context.Context, o *Operation) error
	Get(ctx context.Context, id shared.ID) (*Operation, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Operation, error)
	List(ctx context.Context, filter Filter, page shared.Page) ([]*Operation, error)
	Update(ctx context.Context, o *Operation) error
}

// Filter narrows an operation list query.
type Filter struct {
	ClusterID *shared.ID
	MachineID *shared.ID
	Kind      Kind
	Status    Status
}
