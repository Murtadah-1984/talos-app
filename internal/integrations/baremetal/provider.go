// Package baremetal implements the ports.InfrastructureProvider adapter for
// pre-existing physical hardware (§11, ADR-0007). Unlike a cloud/VM provider,
// bare metal machines are not created by this provider — they are registered
// (discovered or manually enrolled) and then power-managed via a pluggable,
// per-protocol PowerController (see ipmi.go, redfish.go); NoopPowerController
// reports NOT_CONFIGURED for any protocol without a registered controller,
// rather than silently no-op-ing power requests.
package baremetal

import (
	"context"
	"sync"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/machine"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// PowerController performs out-of-band power operations against a machine's
// BMC over one specific protocol. Deliberately decoupled from Talos concerns
// (ADR-0007, §11); Provider dispatches to the controller registered for a
// given machine's declared BMC protocol.
type PowerController interface {
	PowerOn(ctx context.Context, bmcAddress, credentialRef string) error
	PowerOff(ctx context.Context, bmcAddress, credentialRef string) error
	Reboot(ctx context.Context, bmcAddress, credentialRef string) error
	Capability() shared.CapabilityState
}

// NoopPowerController is the fallback for any BMC protocol without a
// registered controller; it reports NOT_CONFIGURED truthfully instead of
// silently no-op-ing power requests.
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
	Protocol      machine.BMCProtocol
	BMCAddress    string
	CredentialRef string
	ManagementIP  string
}

// Provider is the bare-metal InfrastructureProvider. It never creates or
// destroys physical hardware: ProvisionMachine registers an already-existing
// machine into inventory, and DeleteMachine removes it from inventory only.
// Power operations dispatch to the PowerController registered for each
// machine's own declared protocol — a single site's inventory can mix IPMI
// and Redfish machines.
type Provider struct {
	controllers map[machine.BMCProtocol]PowerController

	mu        sync.RWMutex
	inventory map[string]InventoryEntry
}

// NewProvider constructs a bare-metal provider from a protocol -> controller
// registry. A protocol with no entry (or an explicitly nil one) falls back
// to NoopPowerController, so an unconfigured protocol fails loudly
// (NOT_CONFIGURED) rather than silently.
func NewProvider(controllers map[machine.BMCProtocol]PowerController) *Provider {
	return &Provider{controllers: controllers, inventory: make(map[string]InventoryEntry)}
}

func (p *Provider) controllerFor(protocol machine.BMCProtocol) PowerController {
	if c, ok := p.controllers[protocol]; ok && c != nil {
		return c
	}
	return NoopPowerController{}
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
	return p.controllerFor(e.Protocol).PowerOn(ctx, e.BMCAddress, e.CredentialRef)
}

func (p *Provider) PowerOff(ctx context.Context, id string) error {
	e, err := p.entry(id)
	if err != nil {
		return err
	}
	return p.controllerFor(e.Protocol).PowerOff(ctx, e.BMCAddress, e.CredentialRef)
}

func (p *Provider) Reboot(ctx context.Context, id string) error {
	e, err := p.entry(id)
	if err != nil {
		return err
	}
	return p.controllerFor(e.Protocol).Reboot(ctx, e.BMCAddress, e.CredentialRef)
}

func (p *Provider) GetMachineStatus(_ context.Context, id string) (ports.MachineStatusInfra, error) {
	_, err := p.entry(id)
	if err != nil {
		return ports.MachineStatusInfra{}, err
	}
	return ports.MachineStatusInfra{Exists: true, PowerOn: true}, nil
}

// Capability reports SupportsPowerControl true if at least one registered
// protocol controller is actually available — a mixed inventory (e.g. some
// IPMI machines configured, Redfish not yet) is still meaningfully usable.
func (p *Provider) Capability() ports.ProviderCapability {
	powerAvailable := false
	for _, c := range p.controllers {
		if c != nil && c.Capability() == shared.CapabilityAvailable {
			powerAvailable = true
			break
		}
	}
	return ports.ProviderCapability{
		State:                shared.CapabilityAvailable,
		SupportsDiscovery:    true,
		SupportsProvision:    false,
		SupportsPowerControl: powerAvailable,
	}
}
