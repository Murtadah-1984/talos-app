// Package machineservice implements imperative machine operations (§11,
// §17) as Operation records. Two distinct action families exist: soft
// operations (reboot/shutdown/upgrade) go through the TalosClient port
// (ADR-0001) and ask the OS to act; hard power operations go through the
// resolved ports.InfrastructureProvider (ADR-0007) — a BMC (IPMI/Redfish)
// or hypervisor (Proxmox) API — and work even when Talos itself is
// unresponsive. Declarative machine assignment (which cluster/role a
// machine has) is plain CRUD against machine.Repository.
package machineservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/inframanager"
	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/infraprovider"
	"github.com/talos-platform/talos-platform/internal/domain/machine"
	"github.com/talos-platform/talos-platform/internal/domain/operation"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type Service struct {
	machines       machine.Repository
	operations     operation.Repository
	talos          ports.TalosClient
	infraProviders infraprovider.Repository
	registry       *inframanager.Registry
}

func New(machines machine.Repository, operations operation.Repository, talos ports.TalosClient, infraProviders infraprovider.Repository, registry *inframanager.Registry) *Service {
	return &Service{machines: machines, operations: operations, talos: talos, infraProviders: infraProviders, registry: registry}
}

func (s *Service) Get(ctx context.Context, id shared.ID) (*machine.Machine, error) {
	return s.machines.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, filter machine.Filter, page shared.Page) ([]*machine.Machine, error) {
	return s.machines.List(ctx, filter, page)
}

// Health queries the machine's live Talos health, distinct from the
// persisted machine.Phase (§17, "View Health").
func (s *Service) Health(ctx context.Context, id shared.ID) (ports.HealthStatus, error) {
	m, err := s.machines.Get(ctx, id)
	if err != nil {
		return ports.HealthStatus{}, err
	}
	return s.talos.GetHealth(ctx, m.ManagementIP)
}

// beginOperation records an idempotent, RUNNING Operation for machineID and
// returns it (or the already-recorded terminal Operation for a repeated
// idempotency key, in which case ok is false and callers must not run fn
// again). Shared setup for both the Talos-backed and infrastructure-
// provider-backed operation families below.
func (s *Service) beginOperation(ctx context.Context, kind operation.Kind, machineID, requestedBy shared.ID, idempotencyKey string) (op *operation.Operation, m *machine.Machine, ok bool, err error) {
	if existing, err := s.operations.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return existing, nil, false, nil
	} else if !errors.Is(err, shared.ErrNotFound) {
		return nil, nil, false, fmt.Errorf("checking idempotency key: %w", err)
	}

	m, err = s.machines.Get(ctx, machineID)
	if err != nil {
		return nil, nil, false, err
	}

	now := time.Now().UTC()
	op = &operation.Operation{
		ID:             shared.NewID(),
		IdempotencyKey: idempotencyKey,
		Kind:           kind,
		MachineID:      &machineID,
		ClusterID:      m.ClusterID,
		RequestedBy:    requestedBy,
		Status:         operation.StatusRunning,
		StartedAt:      &now,
	}
	if err := s.operations.Create(ctx, op); err != nil {
		return nil, nil, false, fmt.Errorf("recording operation: %w", err)
	}
	return op, m, true, nil
}

func (s *Service) finishOperation(ctx context.Context, op *operation.Operation, runErr error) (*operation.Operation, error) {
	finished := time.Now().UTC()
	op.FinishedAt = &finished
	if runErr != nil {
		op.Status = operation.StatusFailed
		op.Result = runErr.Error()
	} else {
		op.Status = operation.StatusSucceeded
		op.Result = "ok"
	}
	if err := s.operations.Update(ctx, op); err != nil {
		return nil, fmt.Errorf("persisting operation result: %w", err)
	}
	if runErr != nil {
		return op, fmt.Errorf("%s failed: %w", op.Kind, runErr)
	}
	return op, nil
}

// runOperation executes fn against the machine's Talos endpoint (ADR-0001).
func (s *Service) runOperation(ctx context.Context, kind operation.Kind, machineID, requestedBy shared.ID, idempotencyKey string, fn func(ctx context.Context, endpoint string) error) (*operation.Operation, error) {
	op, m, ok, err := s.beginOperation(ctx, kind, machineID, requestedBy, idempotencyKey)
	if err != nil || !ok {
		return op, err
	}
	return s.finishOperation(ctx, op, fn(ctx, m.ManagementIP))
}

// runInfraOperation resolves machineID's infrastructure provider (via its
// registered infraprovider.InfrastructureProvider row's Type) and executes
// fn against it (ADR-0007) — used for hard power control, which must work
// even when Talos/the OS is unresponsive.
func (s *Service) runInfraOperation(ctx context.Context, kind operation.Kind, machineID, requestedBy shared.ID, idempotencyKey string, fn func(ctx context.Context, provider ports.InfrastructureProvider, providerMachineID string) error) (*operation.Operation, error) {
	op, m, ok, err := s.beginOperation(ctx, kind, machineID, requestedBy, idempotencyKey)
	if err != nil || !ok {
		return op, err
	}

	providerRow, err := s.infraProviders.Get(ctx, m.ProviderID)
	if err != nil {
		return s.finishOperation(ctx, op, fmt.Errorf("resolving infrastructure provider record: %w", err))
	}
	provider, err := s.registry.For(providerRow.Type)
	if err != nil {
		return s.finishOperation(ctx, op, err)
	}

	return s.finishOperation(ctx, op, fn(ctx, provider, m.ProviderMachineID))
}

func (s *Service) Reboot(ctx context.Context, machineID, requestedBy shared.ID, idempotencyKey string) (*operation.Operation, error) {
	return s.runOperation(ctx, operation.KindRebootMachine, machineID, requestedBy, idempotencyKey, s.talos.Reboot)
}

func (s *Service) Shutdown(ctx context.Context, machineID, requestedBy shared.ID, idempotencyKey string) (*operation.Operation, error) {
	return s.runOperation(ctx, operation.KindShutdownMachine, machineID, requestedBy, idempotencyKey, s.talos.Shutdown)
}

func (s *Service) Upgrade(ctx context.Context, machineID, requestedBy shared.ID, idempotencyKey, image string) (*operation.Operation, error) {
	return s.runOperation(ctx, operation.KindUpgradeTalos, machineID, requestedBy, idempotencyKey, func(ctx context.Context, endpoint string) error {
		return s.talos.Upgrade(ctx, endpoint, ports.UpgradeOptions{Image: image})
	})
}

// HardPowerOn powers a machine on via its infrastructure provider's BMC/
// hypervisor API — useful when the machine is fully off and Talos has
// nothing to talk to.
func (s *Service) HardPowerOn(ctx context.Context, machineID, requestedBy shared.ID, idempotencyKey string) (*operation.Operation, error) {
	return s.runInfraOperation(ctx, operation.KindHardPowerOn, machineID, requestedBy, idempotencyKey, func(ctx context.Context, provider ports.InfrastructureProvider, id string) error {
		return provider.PowerOn(ctx, id)
	})
}

// HardPowerOff forces a machine off via its infrastructure provider,
// bypassing any graceful OS shutdown — use Shutdown instead when the OS is
// responsive.
func (s *Service) HardPowerOff(ctx context.Context, machineID, requestedBy shared.ID, idempotencyKey string) (*operation.Operation, error) {
	return s.runInfraOperation(ctx, operation.KindHardPowerOff, machineID, requestedBy, idempotencyKey, func(ctx context.Context, provider ports.InfrastructureProvider, id string) error {
		return provider.PowerOff(ctx, id)
	})
}

// HardPowerCycle forces a hard reset via the infrastructure provider — use
// Reboot instead when the OS is responsive and a graceful restart suffices.
func (s *Service) HardPowerCycle(ctx context.Context, machineID, requestedBy shared.ID, idempotencyKey string) (*operation.Operation, error) {
	return s.runInfraOperation(ctx, operation.KindHardPowerCycle, machineID, requestedBy, idempotencyKey, func(ctx context.Context, provider ports.InfrastructureProvider, id string) error {
		return provider.Reboot(ctx, id)
	})
}
