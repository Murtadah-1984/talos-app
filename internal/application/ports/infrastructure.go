package ports

import (
	"context"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// MachineSpec describes the machine an InfrastructureProvider should create.
type MachineSpec struct {
	Hostname    string
	CPU         int32
	MemoryBytes int64
	DiskBytes   int64
	Networks    []string
	Image       string // Talos image/template reference
	CloudInit   string
}

// ProvisionedMachine is what a provider returns after creating a machine.
type ProvisionedMachine struct {
	ProviderMachineID string
	ManagementIP      string
}

// MachineStatusInfra is the infra-level (power/existence) status of a machine,
// distinct from ports.MachineStatus which is Talos-level.
type MachineStatusInfra struct {
	Exists  bool
	PowerOn bool
}

// ProviderCapability declares which operations an InfrastructureProvider
// implementation actually supports, so callers can show NOT_SUPPORTED instead
// of guessing (ADR-0007).
type ProviderCapability struct {
	State                shared.CapabilityState
	SupportsDiscovery    bool
	SupportsProvision    bool
	SupportsPowerControl bool
}

// InfrastructureProvider is implemented independently per provider type
// (bare metal, Proxmox, ...) and resolved via a registry keyed on
// infraprovider.Type (ADR-0007).
type InfrastructureProvider interface {
	DiscoverMachines(ctx context.Context) ([]ProvisionedMachine, error)
	ProvisionMachine(ctx context.Context, spec MachineSpec) (ProvisionedMachine, error)
	DeleteMachine(ctx context.Context, providerMachineID string) error
	PowerOn(ctx context.Context, providerMachineID string) error
	PowerOff(ctx context.Context, providerMachineID string) error
	Reboot(ctx context.Context, providerMachineID string) error
	GetMachineStatus(ctx context.Context, providerMachineID string) (MachineStatusInfra, error)
	Capability() ProviderCapability
}
