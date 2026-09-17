// Package baremetal implements the ports.InfrastructureProvider adapter for
// pre-existing physical hardware (§11, ADR-0007). Unlike a cloud/VM provider,
// bare metal machines are not created by this provider — they are registered
// (discovered or manually enrolled) and then power-managed via a pluggable
// PowerController. IPMI/Redfish implementations of PowerController land in a
// later phase; NoopPowerController reports NOT_CONFIGURED until one is wired.
package baremetal

import (
	"context"
	"sync"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// PowerController performs out-of-band power operations against a machine's
// BMC. Deliberately decoupled from Talos concerns (ADR-0007, §11).
type PowerController interface {
	PowerOn(ctx context.Context, bmcAddress, credentialRef string) error
	PowerOff(ctx context.Context, bmcAddress, credentialRef string) error
	Reboot(ctx context.Context, bmcAddress, credentialRef string) error
	Capability() shared.CapabilityState
}

// NoopPowerController is the default when no IPMI/Redfish backend is
// configured; it reports NOT_CONFIGURED truthfully instead of silently
// no-op-ing power requests.
type NoopPowerController struct{}

func (NoopPowerController) PowerOn(context.Context, string, string) error {
	return shared.ErrCapabilityUnmet
}
func (NoopPowerController) PowerOff(context.Context, string, string) error {
	return shared.ErrCapabilityUnmet
}
func (NoopPowerController) Reboot(context.Context, string, string) error {
	return shared.ErrCapabilityUnmet
}
func (NoopPowerController) Capability() shared.CapabilityState {
	return shared.CapabilityNotConfigured
}

// InventoryEntry is a registered physical machine, keyed by its management
// (BMC or OS) address rather than a cloud-style instance ID.
type InventoryEntry struct {
	BMCAddress    string
	CredentialRef string
	ManagementIP  string
}

// Provider is the bare-metal InfrastructureProvider. It never creates or
// destroys physical hardware: ProvisionMachine registers an already-existing
// machine into inventory, and DeleteMachine removes it from inventory only.
type Provider struct {
	power PowerController

	mu        sync.RWMutex
	inventory map[string]InventoryEntry
}

// NewProvider constructs a bare-metal provider. Pass NoopPowerController{}
// until an IPMI/Redfish implementation is configured.
func NewProvider(power PowerController) *Provider {
	return &Provider{power: power, inventory: make(map[string]InventoryEntry)}
}

// Register enrolls a physical machine discovered out-of-band (e.g. via a PXE
// boot callback or manual operator entry) into the provider's inventory.
func (p *Provider) Register(id string, entry InventoryEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inventory[id] = entry
}

func (p *Provider) DiscoverMachines(_ context.Context) ([]ports.ProvisionedMachine, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]ports.ProvisionedMachine, 0, len(p.inventory))
	for id, e := range p.inventory {
		out = append(out, ports.ProvisionedMachine{ProviderMachineID: id, ManagementIP: e.ManagementIP})
	}
	return out, nil
}

// ProvisionMachine on bare metal means "claim an already-enrolled machine";
// it never fabricates hardware. Callers must Register the machine first.
func (p *Provider) ProvisionMachine(_ context.Context, spec ports.MachineSpec) (ports.ProvisionedMachine, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for id, e := range p.inventory {
		if e.ManagementIP == "" {
			continue
		}
		return ports.ProvisionedMachine{ProviderMachineID: id, ManagementIP: e.ManagementIP}, nil
	}
	_ = spec
	return ports.ProvisionedMachine{}, shared.ErrNotFound
}

func (p *Provider) DeleteMachine(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.inventory[id]; !ok {
		return shared.ErrNotFound
	}
	delete(p.inventory, id)
	return nil
}

func (p *Provider) entry(id string) (InventoryEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.inventory[id]
	if !ok {
		return InventoryEntry{}, shared.ErrNotFound
	}
	return e, nil
}

func (p *Provider) PowerOn(ctx context.Context, id string) error {
	e, err := p.entry(id)
	if err != nil {
		return err
	}
	return p.power.PowerOn(ctx, e.BMCAddress, e.CredentialRef)
}

func (p *Provider) PowerOff(ctx context.Context, id string) error {
	e, err := p.entry(id)
	if err != nil {
		return err
	}
	return p.power.PowerOff(ctx, e.BMCAddress, e.CredentialRef)
}

func (p *Provider) Reboot(ctx context.Context, id string) error {
	e, err := p.entry(id)
	if err != nil {
		return err
	}
	return p.power.Reboot(ctx, e.BMCAddress, e.CredentialRef)
}

func (p *Provider) GetMachineStatus(_ context.Context, id string) (ports.MachineStatusInfra, error) {
	_, err := p.entry(id)
	if err != nil {
		return ports.MachineStatusInfra{}, err
	}
	return ports.MachineStatusInfra{Exists: true, PowerOn: true}, nil
}

func (p *Provider) Capability() ports.ProviderCapability {
	return ports.ProviderCapability{
		State:                shared.CapabilityAvailable,
		SupportsDiscovery:    true,
		SupportsProvision:    false,
		SupportsPowerControl: p.power.Capability() == shared.CapabilityAvailable,
	}
}
