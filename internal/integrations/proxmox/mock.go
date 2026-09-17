// Package proxmox implements the ports.InfrastructureProvider adapter for
// Proxmox VE (§12, ADR-0007). The real client (Proxmox REST API: cluster/node/
// VM discovery, VM lifecycle, cloud-init, Talos image deployment) lands in
// Phase 6; this mock lets cluster provisioning workflows run against a
// realistic VM lifecycle without a real Proxmox cluster.
package proxmox

import (
	"context"
	"fmt"
	"sync"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type MockProvider struct {
	mu  sync.Mutex
	vms map[string]*vmState
	seq int
}

type vmState struct {
	exists  bool
	powerOn bool
	ip      string
}

func NewMockProvider() *MockProvider {
	return &MockProvider{vms: make(map[string]*vmState)}
}

func (p *MockProvider) DiscoverMachines(_ context.Context) ([]ports.ProvisionedMachine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []ports.ProvisionedMachine
	for id, vm := range p.vms {
		if vm.exists {
			out = append(out, ports.ProvisionedMachine{ProviderMachineID: id, ManagementIP: vm.ip})
		}
	}
	return out, nil
}

func (p *MockProvider) ProvisionMachine(_ context.Context, spec ports.MachineSpec) (ports.ProvisionedMachine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seq++
	id := fmt.Sprintf("mock-vm-%d", 100+p.seq)
	ip := fmt.Sprintf("10.10.%d.%d", p.seq/254+1, p.seq%254+1)
	p.vms[id] = &vmState{exists: true, powerOn: true, ip: ip}
	_ = spec.Hostname
	return ports.ProvisionedMachine{ProviderMachineID: id, ManagementIP: ip}, nil
}

func (p *MockProvider) DeleteMachine(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.vms[id]; !ok {
		return shared.ErrNotFound
	}
	delete(p.vms, id)
	return nil
}

func (p *MockProvider) PowerOn(_ context.Context, id string) error  { return p.setPower(id, true) }
func (p *MockProvider) PowerOff(_ context.Context, id string) error { return p.setPower(id, false) }

func (p *MockProvider) setPower(id string, on bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	vm, ok := p.vms[id]
	if !ok {
		return shared.ErrNotFound
	}
	vm.powerOn = on
	return nil
}

func (p *MockProvider) Reboot(ctx context.Context, id string) error {
	if err := p.PowerOff(ctx, id); err != nil {
		return err
	}
	return p.PowerOn(ctx, id)
}

func (p *MockProvider) GetMachineStatus(_ context.Context, id string) (ports.MachineStatusInfra, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	vm, ok := p.vms[id]
	if !ok {
		return ports.MachineStatusInfra{}, shared.ErrNotFound
	}
	return ports.MachineStatusInfra{Exists: vm.exists, PowerOn: vm.powerOn}, nil
}

func (p *MockProvider) Capability() ports.ProviderCapability {
	return ports.ProviderCapability{
		State:                shared.CapabilityAvailable,
		SupportsDiscovery:    true,
		SupportsProvision:    true,
		SupportsPowerControl: true,
	}
}
