package machineservice_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/inframanager"
	"github.com/talos-platform/talos-platform/internal/application/machineservice"
	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/infraprovider"
	"github.com/talos-platform/talos-platform/internal/domain/machine"
	"github.com/talos-platform/talos-platform/internal/domain/operation"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/infrastructure/secrets"
	"github.com/talos-platform/talos-platform/internal/integrations/talos"
)

// fakeClusterRepo is a minimal in-memory cluster.Repository covering only
// what ensureTalosCredentials touches (Get).
type fakeClusterRepo struct {
	cluster.Repository
	clusters map[shared.ID]*cluster.Cluster
}

func newFakeClusterRepo(clusters ...*cluster.Cluster) *fakeClusterRepo {
	r := &fakeClusterRepo{clusters: make(map[shared.ID]*cluster.Cluster)}
	for _, c := range clusters {
		r.clusters[c.ID] = c
	}
	return r
}

func (r *fakeClusterRepo) Get(_ context.Context, id shared.ID) (*cluster.Cluster, error) {
	c, ok := r.clusters[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return c, nil
}

// fakeMachineRepo is a minimal in-memory machine.Repository.
type fakeMachineRepo struct {
	machine.Repository
	machines map[shared.ID]*machine.Machine
}

func newFakeMachineRepo(machines ...*machine.Machine) *fakeMachineRepo {
	r := &fakeMachineRepo{machines: make(map[shared.ID]*machine.Machine)}
	for _, m := range machines {
		r.machines[m.ID] = m
	}
	return r
}

func (r *fakeMachineRepo) Get(_ context.Context, id shared.ID) (*machine.Machine, error) {
	m, ok := r.machines[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return m, nil
}

// fakeOperationRepo is a minimal in-memory operation.Repository.
type fakeOperationRepo struct {
	operation.Repository
	mu    sync.Mutex
	byID  map[shared.ID]*operation.Operation
	byKey map[string]shared.ID
}

func newFakeOperationRepo() *fakeOperationRepo {
	return &fakeOperationRepo{byID: make(map[shared.ID]*operation.Operation), byKey: make(map[string]shared.ID)}
}

func (r *fakeOperationRepo) Create(_ context.Context, o *operation.Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[o.ID] = o
	r.byKey[o.IdempotencyKey] = o.ID
	return nil
}

func (r *fakeOperationRepo) GetByIdempotencyKey(_ context.Context, key string) (*operation.Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byKey[key]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return r.byID[id], nil
}

func (r *fakeOperationRepo) Update(_ context.Context, o *operation.Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[o.ID] = o
	return nil
}

// fakeInfraProviderRepo is a minimal in-memory infraprovider.Repository.
type fakeInfraProviderRepo struct {
	infraprovider.Repository
	providers map[shared.ID]*infraprovider.InfrastructureProvider
}

func (r *fakeInfraProviderRepo) Get(_ context.Context, id shared.ID) (*infraprovider.InfrastructureProvider, error) {
	p, ok := r.providers[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return p, nil
}

// fakeInfraProvider is a minimal ports.InfrastructureProvider recording
// which calls it received.
type fakeInfraProvider struct {
	ports.InfrastructureProvider
	powerOnCalls, powerOffCalls, rebootCalls []string
	err                                      error
}

func (f *fakeInfraProvider) PowerOn(_ context.Context, id string) error {
	f.powerOnCalls = append(f.powerOnCalls, id)
	return f.err
}
func (f *fakeInfraProvider) PowerOff(_ context.Context, id string) error {
	f.powerOffCalls = append(f.powerOffCalls, id)
	return f.err
}
func (f *fakeInfraProvider) Reboot(_ context.Context, id string) error {
	f.rebootCalls = append(f.rebootCalls, id)
	return f.err
}

func setup(t *testing.T) (*machineservice.Service, *machine.Machine, *fakeInfraProvider) {
	t.Helper()
	providerRowID := shared.NewID()
	m := &machine.Machine{
		ID:                shared.NewID(),
		ProviderID:        providerRowID,
		ProviderMachineID: "104",
		Hostname:          "worker-1",
	}
	infra := &fakeInfraProvider{}
	infraProviders := &fakeInfraProviderRepo{providers: map[shared.ID]*infraprovider.InfrastructureProvider{
		providerRowID: {ID: providerRowID, Type: infraprovider.TypeProxmox},
	}}
	registry := inframanager.NewRegistry(map[infraprovider.Type]ports.InfrastructureProvider{
		infraprovider.TypeProxmox: infra,
	})
	svc := machineservice.New(newFakeMachineRepo(m), newFakeOperationRepo(), nil, infraProviders, registry, nil, nil)
	return svc, m, infra
}

func TestMachineService_HardPowerOn_CallsResolvedProviderWithProviderMachineID(t *testing.T) {
	svc, m, infra := setup(t)

	op, err := svc.HardPowerOn(context.Background(), m.ID, shared.NewID(), "power-on-key")
	if err != nil {
		t.Fatalf("HardPowerOn: %v", err)
	}
	if op.Status != operation.StatusSucceeded {
		t.Fatalf("expected SUCCEEDED, got %s (result: %s)", op.Status, op.Result)
	}
	if len(infra.powerOnCalls) != 1 || infra.powerOnCalls[0] != "104" {
		t.Fatalf("expected PowerOn called with providerMachineID 104, got %v", infra.powerOnCalls)
	}
	if op.Kind != operation.KindHardPowerOn {
		t.Errorf("expected operation kind HARD_POWER_ON, got %s", op.Kind)
	}
}

func TestMachineService_HardPowerOff_RecordsFailureWhenProviderErrors(t *testing.T) {
	svc, m, infra := setup(t)
	infra.err = errors.New("bmc unreachable")

	op, err := svc.HardPowerOff(context.Background(), m.ID, shared.NewID(), "power-off-key")
	if err == nil {
		t.Fatal("expected an error when the provider call fails")
	}
	if op.Status != operation.StatusFailed {
		t.Fatalf("expected FAILED, got %s", op.Status)
	}
	if op.Result != "bmc unreachable" {
		t.Errorf("expected result to capture the underlying error, got %q", op.Result)
	}
}

func TestMachineService_HardPowerCycle_IsIdempotent(t *testing.T) {
	svc, m, infra := setup(t)

	first, err := svc.HardPowerCycle(context.Background(), m.ID, shared.NewID(), "cycle-key")
	if err != nil {
		t.Fatalf("first HardPowerCycle: %v", err)
	}
	second, err := svc.HardPowerCycle(context.Background(), m.ID, shared.NewID(), "cycle-key")
	if err != nil {
		t.Fatalf("second HardPowerCycle: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the same operation for a repeated idempotency key, got %s and %s", first.ID, second.ID)
	}
	if len(infra.rebootCalls) != 1 {
		t.Fatalf("expected the provider to be called exactly once, got %d calls", len(infra.rebootCalls))
	}
}

func TestMachineService_HardPowerOn_UnknownProviderType(t *testing.T) {
	providerRowID := shared.NewID()
	m := &machine.Machine{ID: shared.NewID(), ProviderID: providerRowID, ProviderMachineID: "1"}
	infraProviders := &fakeInfraProviderRepo{providers: map[shared.ID]*infraprovider.InfrastructureProvider{
		providerRowID: {ID: providerRowID, Type: infraprovider.TypeAWS},
	}}
	registry := inframanager.NewRegistry(map[infraprovider.Type]ports.InfrastructureProvider{})
	svc := machineservice.New(newFakeMachineRepo(m), newFakeOperationRepo(), nil, infraProviders, registry, nil, nil)

	op, err := svc.HardPowerOn(context.Background(), m.ID, shared.NewID(), "key")
	if err == nil {
		t.Fatal("expected an error for a provider type with no registered implementation")
	}
	if op.Status != operation.StatusFailed {
		t.Fatalf("expected the operation to be recorded as FAILED, got %s", op.Status)
	}
}

func TestMachineService_DiscoverClusterTopology(t *testing.T) {
	svc := machineservice.New(newFakeMachineRepo(), newFakeOperationRepo(), talos.NewMockClient(), &fakeInfraProviderRepo{providers: map[shared.ID]*infraprovider.InfrastructureProvider{}}, inframanager.NewRegistry(nil), nil, nil)

	members, err := svc.DiscoverClusterTopology(context.Background(), "10.0.0.5")
	if err != nil {
		t.Fatalf("DiscoverClusterTopology: %v", err)
	}
	if len(members) != 1 || members[0].Hostname != "10.0.0.5" || !members[0].ControlPlane {
		t.Fatalf("unexpected discovered members: %+v", members)
	}
}

func TestMachineService_Health_ResolvesPerClusterTalosCredentials(t *testing.T) {
	ctx := context.Background()
	secretStore, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	talosconfig := []byte("context: cluster-a\ncontexts:\n  cluster-a: {}\n")
	ref := ports.SecretRef{Backend: "talos", Path: "cluster-a-talosconfig"}
	if err := secretStore.Put(ctx, ref, talosconfig); err != nil {
		t.Fatalf("seeding secret store: %v", err)
	}

	c := &cluster.Cluster{ID: shared.NewID(), Name: "cluster-a", TalosConfigRef: "cluster-a-talosconfig"}
	clusters := newFakeClusterRepo(c)

	m := &machine.Machine{ID: shared.NewID(), ClusterID: &c.ID, ManagementIP: "10.0.0.5"}
	talosMock := talos.NewMockClient()

	svc := machineservice.New(newFakeMachineRepo(m), newFakeOperationRepo(), talosMock, &fakeInfraProviderRepo{providers: map[shared.ID]*infraprovider.InfrastructureProvider{}}, inframanager.NewRegistry(nil), clusters, secretStore)

	if _, err := svc.Health(ctx, m.ID); err != nil {
		t.Fatalf("Health: %v", err)
	}

	got, ok := talosMock.CredentialsFor(m.ManagementIP)
	if !ok {
		t.Fatal("expected EnsureCredentials to have been called for the machine's endpoint")
	}
	if string(got) != string(talosconfig) {
		t.Errorf("expected the cluster's own talosconfig to be registered, got %q", got)
	}
}

func TestMachineService_Health_FallsBackToDefaultCredentialsWhenClusterHasNoConfigRef(t *testing.T) {
	ctx := context.Background()
	secretStore, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}

	c := &cluster.Cluster{ID: shared.NewID(), Name: "cluster-a"} // no TalosConfigRef
	clusters := newFakeClusterRepo(c)
	m := &machine.Machine{ID: shared.NewID(), ClusterID: &c.ID, ManagementIP: "10.0.0.5"}
	talosMock := talos.NewMockClient()

	svc := machineservice.New(newFakeMachineRepo(m), newFakeOperationRepo(), talosMock, &fakeInfraProviderRepo{providers: map[shared.ID]*infraprovider.InfrastructureProvider{}}, inframanager.NewRegistry(nil), clusters, secretStore)

	if _, err := svc.Health(ctx, m.ID); err != nil {
		t.Fatalf("Health: %v", err)
	}

	if _, ok := talosMock.CredentialsFor(m.ManagementIP); ok {
		t.Error("expected no EnsureCredentials call when the cluster has no TalosConfigRef (falls back to the default talosconfig)")
	}
}
