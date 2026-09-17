// Package machineservice implements imperative machine operations (§11,
// §17) as Operation records executed through the TalosClient port
// (ADR-0001). Declarative machine assignment (which cluster/role a machine
// has) is plain CRUD against machine.Repository; only reboot/shutdown/
// upgrade go through Talos.
package machineservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/machine"
	"github.com/talos-platform/talos-platform/internal/domain/operation"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type Service struct {
	machines   machine.Repository
	operations operation.Repository
	talos      ports.TalosClient
}

func New(machines machine.Repository, operations operation.Repository, talos ports.TalosClient) *Service {
	return &Service{machines: machines, operations: operations, talos: talos}
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

// runOperation records an idempotent Operation and executes fn against the
// machine's Talos endpoint, updating the operation's terminal status (§17,
// §32). Dangerous kinds (operation.Kind.Danger()) must already have been
// confirmed by the caller (the HTTP layer enforces this before invoking).
func (s *Service) runOperation(ctx context.Context, kind operation.Kind, machineID, requestedBy shared.ID, idempotencyKey string, fn func(ctx context.Context, endpoint string) error) (*operation.Operation, error) {
	if existing, err := s.operations.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("checking idempotency key: %w", err)
	}

	m, err := s.machines.Get(ctx, machineID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	op := &operation.Operation{
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
		return nil, fmt.Errorf("recording operation: %w", err)
	}

	runErr := fn(ctx, m.ManagementIP)
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
		return op, fmt.Errorf("%s failed: %w", kind, runErr)
	}
	return op, nil
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
