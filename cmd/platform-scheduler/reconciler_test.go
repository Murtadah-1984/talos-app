package main

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/integrations/argocd"
	"github.com/talos-platform/talos-platform/internal/observability"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeAuditRepo is a minimal in-memory audit.Repository covering only what
// the reconciler touches: alert upsert/list.
type fakeAuditRepo struct {
	audit.Repository
	mu     sync.Mutex
	alerts map[string]*audit.Alert // keyed by targetKind+targetID+title
}

func newFakeAuditRepo() *fakeAuditRepo {
	return &fakeAuditRepo{alerts: make(map[string]*audit.Alert)}
}

func alertKey(kind, id, title string) string { return kind + "|" + id + "|" + title }

func (f *fakeAuditRepo) UpsertAlert(_ context.Context, a *audit.Alert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := alertKey(a.TargetKind, a.TargetID, a.Title)
	f.alerts[key] = a
	return nil
}

func (f *fakeAuditRepo) ListAlerts(_ context.Context, filter audit.AlertFilter, _ shared.Page) ([]*audit.Alert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*audit.Alert
	for _, a := range f.alerts {
		if filter.TargetKind != "" && a.TargetKind != filter.TargetKind {
			continue
		}
		if filter.TargetID != "" && a.TargetID != filter.TargetID {
			continue
		}
		if filter.Status != "" && a.Status != filter.Status {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// fakeGitOpsRepo is a minimal in-memory gitops.Repositories covering only
// UpsertArgoApplication, which is all the reconciler calls.
type fakeGitOpsRepo struct {
	gitops.Repositories
	mu   sync.Mutex
	apps map[shared.ID]*gitops.ArgoApplication
}

func newFakeGitOpsRepo() *fakeGitOpsRepo {
	return &fakeGitOpsRepo{apps: make(map[shared.ID]*gitops.ArgoApplication)}
}

func (f *fakeGitOpsRepo) UpsertArgoApplication(_ context.Context, a *gitops.ArgoApplication) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.apps[a.ClusterID] = a
	return nil
}

func newTestReconciler(gitopsRepo *fakeGitOpsRepo, auditRepo *fakeAuditRepo, argo *argocd.MockClient) *reconciler {
	return &reconciler{
		gitops:  gitopsRepo,
		audit:   auditRepo,
		argo:    argo,
		metrics: observability.NewMetrics(),
		logger:  discardLogger(),
	}
}

func testCluster(argoEnabled bool) *cluster.Cluster {
	return &cluster.Cluster{
		ID:    shared.NewID(),
		Name:  "basra-prod",
		State: cluster.StateReady,
		Spec:  cluster.Spec{ArgoCD: cluster.ArgoCDSpec{Enabled: argoEnabled, Project: "default"}},
	}
}

func TestReconciler_FiresAlertOnDrift(t *testing.T) {
	gitopsRepo := newFakeGitOpsRepo()
	auditRepo := newFakeAuditRepo()
	argo := argocd.NewMockClient()
	c := testCluster(true)
	argo.Seed(c.Name, "argocd", "default", "rev1")
	argo.SetStatus(c.Name, gitops.SyncStatusOutOfSync, gitops.HealthHealthy)

	r := newTestReconciler(gitopsRepo, auditRepo, argo)
	firing, err := r.firingAlertTitles(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("firingAlertTitles: %v", err)
	}
	r.reconcileArgoCD(context.Background(), c, firing)

	alerts, _ := auditRepo.ListAlerts(context.Background(), audit.AlertFilter{TargetKind: "cluster", TargetID: c.ID.String()}, shared.DefaultPage())
	var found bool
	for _, a := range alerts {
		if a.Title == "GitOps drift detected" {
			found = true
			if a.Status != audit.AlertFiring {
				t.Errorf("expected drift alert to be FIRING, got %s", a.Status)
			}
		}
	}
	if !found {
		t.Fatal("expected a 'GitOps drift detected' alert to be recorded")
	}

	cached := gitopsRepo.apps[c.ID]
	if cached == nil || cached.SyncStatus != gitops.SyncStatusOutOfSync {
		t.Errorf("expected the ArgoApplication cache to reflect OutOfSync, got %+v", cached)
	}
}

func TestReconciler_ResolvesAlertWhenDriftClears(t *testing.T) {
	gitopsRepo := newFakeGitOpsRepo()
	auditRepo := newFakeAuditRepo()
	argo := argocd.NewMockClient()
	c := testCluster(true)
	argo.Seed(c.Name, "argocd", "default", "rev1")

	r := newTestReconciler(gitopsRepo, auditRepo, argo)

	// Seed a pre-existing FIRING alert directly, simulating a prior tick
	// that observed drift, to verify the reconciler resolves it once the
	// (mock, always-Synced) Application is observed as in sync again.
	_ = auditRepo.UpsertAlert(context.Background(), &audit.Alert{
		TargetKind: "cluster", TargetID: c.ID.String(), Title: "GitOps drift detected",
		Status: audit.AlertFiring, Severity: audit.SeverityWarning,
	})

	firing, err := r.firingAlertTitles(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("firingAlertTitles: %v", err)
	}
	if !firing["GitOps drift detected"] {
		t.Fatal("expected the seeded alert to be reported as firing")
	}

	r.reconcileArgoCD(context.Background(), c, firing)

	alerts, _ := auditRepo.ListAlerts(context.Background(), audit.AlertFilter{TargetKind: "cluster", TargetID: c.ID.String()}, shared.DefaultPage())
	var found bool
	for _, a := range alerts {
		if a.Title == "GitOps drift detected" {
			found = true
			if a.Status != audit.AlertResolved {
				t.Errorf("expected drift alert to be RESOLVED, got %s", a.Status)
			}
		}
	}
	if !found {
		t.Fatal("expected a 'GitOps drift detected' alert record to exist")
	}
}

func TestReconciler_SkipsArgoCDWhenDisabled(t *testing.T) {
	gitopsRepo := newFakeGitOpsRepo()
	auditRepo := newFakeAuditRepo()
	argo := argocd.NewMockClient()
	c := testCluster(false)

	r := newTestReconciler(gitopsRepo, auditRepo, argo)
	r.reconcileCluster(context.Background(), c)

	if len(gitopsRepo.apps) != 0 {
		t.Errorf("expected no Argo Application cache writes for an ArgoCD-disabled cluster, got %d", len(gitopsRepo.apps))
	}
}
