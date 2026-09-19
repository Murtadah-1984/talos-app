// Package argocd implements the ports.ArgoCDClient adapter (ADR-0003). This
// file is the deterministic mock, used in local development and tests; see
// client.go for the real REST-based implementation (Phase 4).
package argocd

import (
	"context"
	"sync"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type MockClient struct {
	mu      sync.Mutex
	apps    map[string]*ports.ApplicationStatus
	appSets map[string]*ports.ApplicationSetStatus
}

func NewMockClient() *MockClient {
	return &MockClient{apps: make(map[string]*ports.ApplicationStatus), appSets: make(map[string]*ports.ApplicationSetStatus)}
}

// SeedApplicationSet registers an ApplicationSet the mock will report on.
func (c *MockClient) SeedApplicationSet(name, namespace string, resources []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.appSets[name] = &ports.ApplicationSetStatus{Name: name, Namespace: namespace, Resources: resources}
}

// Seed registers (or updates) an Application the mock will report on —
// called by the GitOps workflow steps once a commit/PR has been merged, to
// simulate Argo CD picking up the change.
func (c *MockClient) Seed(name, namespace, project, revision string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apps[name] = &ports.ApplicationStatus{
		Name:         name,
		Namespace:    namespace,
		Project:      project,
		SyncStatus:   gitops.SyncStatusSynced,
		HealthStatus: gitops.HealthHealthy,
		Revision:     revision,
	}
}

// SetStatus overrides a previously-seeded Application's sync/health status,
// so tests can simulate drift or degraded health without a live Argo CD
// instance.
func (c *MockClient) SetStatus(name string, sync gitops.ArgoCDSyncStatus, health gitops.ArgoCDHealthStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a, ok := c.apps[name]; ok {
		a.SyncStatus = sync
		a.HealthStatus = health
	}
}

func (c *MockClient) ListApplications(_ context.Context, project string) ([]ports.ApplicationStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []ports.ApplicationStatus
	for _, a := range c.apps {
		if project == "" || a.Project == project {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (c *MockClient) GetApplication(_ context.Context, name string) (ports.ApplicationStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.apps[name]
	if !ok {
		return ports.ApplicationStatus{}, shared.ErrNotFound
	}
	return *a, nil
}

func (c *MockClient) Sync(_ context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.apps[name]
	if !ok {
		return shared.ErrNotFound
	}
	a.SyncStatus = gitops.SyncStatusSynced
	a.HealthStatus = gitops.HealthHealthy
	return nil
}

func (c *MockClient) GetHistory(_ context.Context, name string) ([]ports.SyncHistoryEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.apps[name]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return []ports.SyncHistoryEntry{{Revision: a.Revision, DeployedAt: time.Now().Format(time.RFC3339), Status: "Succeeded"}}, nil
}

func (c *MockClient) ListApplicationSets(_ context.Context, _ string) ([]ports.ApplicationSetStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ports.ApplicationSetStatus, 0, len(c.appSets))
	for _, a := range c.appSets {
		out = append(out, *a)
	}
	return out, nil
}

func (c *MockClient) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}
