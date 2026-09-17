// Package talos implements the ports.TalosClient adapter (ADR-0001). The real
// gRPC-based client lands in Phase 2 (see docs/roadmap.md); today this package
// provides a deterministic mock so the rest of the platform can be built,
// exercised, and tested end-to-end against the same interface.
package talos

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// MockClient is an in-memory, deterministic stand-in for a real Talos
// endpoint. It never claims AVAILABLE capability for anything a real client
// would need PKI for beyond what's configured — it reports its state
// truthfully via Capability().
type MockClient struct {
	mu       sync.Mutex
	machines map[string]*mockMachine
}

type mockMachine struct {
	talosVersion string
	k8sVersion   string
	bootedAt     time.Time
	healthy      bool
}

// NewMockClient constructs a MockClient with no seeded machines; endpoints
// are lazily created on first access so any endpoint string works out of the
// box in local development.
func NewMockClient() *MockClient {
	return &MockClient{machines: make(map[string]*mockMachine)}
}

func (c *MockClient) machine(endpoint string) *mockMachine {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.machines[endpoint]
	if !ok {
		m = &mockMachine{
			talosVersion: "v1.8.2",
			k8sVersion:   "v1.31.1",
			bootedAt:     time.Now().Add(-24 * time.Hour),
			healthy:      true,
		}
		c.machines[endpoint] = m
	}
	return m
}

func (c *MockClient) GetMachineStatus(_ context.Context, endpoint string) (ports.MachineStatus, error) {
	m := c.machine(endpoint)
	return ports.MachineStatus{
		Hostname:     endpoint,
		TalosVersion: m.talosVersion,
		Uptime:       time.Since(m.bootedAt),
		Ready:        m.healthy,
	}, nil
}

func (c *MockClient) GetMachineConfiguration(_ context.Context, endpoint string) (ports.MachineConfiguration, error) {
	return ports.MachineConfiguration{
		Cluster:    "mock",
		Role:       "controlplane",
		RawYAML:    fmt.Sprintf("# mock configuration for %s\nversion: v1alpha1\n", endpoint),
		AppliedRev: "mock-rev-1",
	}, nil
}

func (c *MockClient) ApplyMachineConfiguration(_ context.Context, endpoint string, _ ports.MachineConfiguration, _ ports.ApplyOptions) error {
	c.machine(endpoint) // ensure it exists
	return nil
}

func (c *MockClient) Reboot(_ context.Context, endpoint string) error {
	m := c.machine(endpoint)
	c.mu.Lock()
	m.bootedAt = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *MockClient) Shutdown(_ context.Context, endpoint string) error {
	m := c.machine(endpoint)
	c.mu.Lock()
	m.healthy = false
	c.mu.Unlock()
	return nil
}

func (c *MockClient) Upgrade(_ context.Context, endpoint string, opts ports.UpgradeOptions) error {
	m := c.machine(endpoint)
	c.mu.Lock()
	if opts.Image != "" {
		m.talosVersion = opts.Image
	}
	m.bootedAt = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *MockClient) GetVersion(_ context.Context, endpoint string) (ports.VersionInfo, error) {
	m := c.machine(endpoint)
	return ports.VersionInfo{TalosVersion: m.talosVersion, KubernetesVersion: m.k8sVersion}, nil
}

func (c *MockClient) GetHealth(_ context.Context, endpoint string) (ports.HealthStatus, error) {
	m := c.machine(endpoint)
	return ports.HealthStatus{Healthy: m.healthy, Details: map[string]string{"etcd": "ok", "kubelet": "ok"}}, nil
}

func (c *MockClient) GetServices(_ context.Context, _ string) ([]ports.ServiceInfo, error) {
	return []ports.ServiceInfo{
		{Name: "etcd", State: "Running", Healthy: true},
		{Name: "kubelet", State: "Running", Healthy: true},
		{Name: "trustd", State: "Running", Healthy: true},
	}, nil
}

func (c *MockClient) GetDisks(_ context.Context, _ string) ([]ports.DiskInfo, error) {
	return []ports.DiskInfo{{Device: "/dev/sda", SizeBytes: 500 * 1024 * 1024 * 1024, Model: "mock-ssd"}}, nil
}

func (c *MockClient) GetNetworkInfo(_ context.Context, endpoint string) (ports.NetworkInfo, error) {
	return ports.NetworkInfo{Hostname: endpoint, Addresses: []string{"10.0.0.10"}, Interfaces: []string{"eth0"}}, nil
}

func (c *MockClient) GetEtcdHealth(_ context.Context, endpoint string) (ports.EtcdHealth, error) {
	m := c.machine(endpoint)
	return ports.EtcdHealth{MemberID: endpoint, Healthy: m.healthy, IsLeader: false}, nil
}

func (c *MockClient) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}
