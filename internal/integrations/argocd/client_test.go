package argocd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/talos-platform/talos-platform/internal/domain/gitops"
)

const testApplicationJSON = `{
	"metadata": {"name": "basra-prod", "namespace": "argocd"},
	"spec": {"project": "default"},
	"status": {
		"sync": {"status": "Synced", "revision": "abc123"},
		"health": {"status": "Healthy"},
		"resources": [
			{"kind": "Deployment", "name": "coredns", "namespace": "kube-system", "health": {"status": "Healthy"}}
		],
		"history": [
			{"revision": "abc123", "deployedAt": "2026-01-01T00:00:00Z"},
			{"revision": "def456", "deployedAt": "2025-12-31T00:00:00Z"}
		]
	}
}`

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client := NewClient(srv.URL, "test-token")
	return client, srv.Close
}

func TestClient_GetApplication(t *testing.T) {
	var gotAuth, gotPath string
	client, closeFn := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testApplicationJSON))
	})
	defer closeFn()

	status, err := client.GetApplication(t.Context(), "basra-prod")
	if err != nil {
		t.Fatalf("GetApplication: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("expected bearer auth header, got %q", gotAuth)
	}
	if gotPath != "/api/v1/applications/basra-prod" {
		t.Errorf("unexpected request path %q", gotPath)
	}

	if status.Name != "basra-prod" || status.Namespace != "argocd" || status.Project != "default" {
		t.Errorf("unexpected identity fields: %+v", status)
	}
	if status.SyncStatus != gitops.SyncStatusSynced {
		t.Errorf("expected Synced, got %s", status.SyncStatus)
	}
	if status.HealthStatus != gitops.HealthHealthy {
		t.Errorf("expected Healthy, got %s", status.HealthStatus)
	}
	if status.Revision != "abc123" {
		t.Errorf("expected revision abc123, got %s", status.Revision)
	}
	if len(status.ResourceTree) != 1 || status.ResourceTree[0].Kind != "Deployment" {
		t.Errorf("expected one Deployment resource node, got %+v", status.ResourceTree)
	}
}

func TestClient_GetApplication_NotFound(t *testing.T) {
	client, closeFn := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer closeFn()

	_, err := client.GetApplication(t.Context(), "does-not-exist")
	if err == nil {
		t.Fatal("expected an error for a missing application")
	}
}

func TestClient_Sync_PostsToSyncEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	client, closeFn := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	defer closeFn()

	if err := client.Sync(t.Context(), "basra-prod"); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v1/applications/basra-prod/sync" {
		t.Errorf("unexpected sync path %q", gotPath)
	}
}

func TestClient_GetHistory(t *testing.T) {
	client, closeFn := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testApplicationJSON))
	})
	defer closeFn()

	history, err := client.GetHistory(t.Context(), "basra-prod")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[0].Revision != "abc123" || history[1].Revision != "def456" {
		t.Errorf("unexpected history order/content: %+v", history)
	}
}

func TestClient_ListApplications_FiltersByProject(t *testing.T) {
	var gotQuery string
	client, closeFn := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[` + testApplicationJSON + `]}`))
	})
	defer closeFn()

	apps, err := client.ListApplications(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListApplications: %v", err)
	}
	if !strings.Contains(gotQuery, "project=default") {
		t.Errorf("expected project filter in query, got %q", gotQuery)
	}
	if len(apps) != 1 || apps[0].Name != "basra-prod" {
		t.Errorf("unexpected applications list: %+v", apps)
	}
}

const testApplicationSetJSON = `{
	"metadata": {"name": "clusters-appset", "namespace": "argocd"},
	"status": {
		"resources": [{"name": "basra-prod"}, {"name": "basra-staging"}],
		"conditions": [{"type": "ResourcesUpToDate", "status": "True", "message": "up to date"}]
	}
}`

func TestClient_ListApplicationSets(t *testing.T) {
	var gotPath, gotQuery string
	client, closeFn := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[` + testApplicationSetJSON + `]}`))
	})
	defer closeFn()

	sets, err := client.ListApplicationSets(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListApplicationSets: %v", err)
	}
	if gotPath != "/api/v1/applicationsets" {
		t.Errorf("unexpected request path %q", gotPath)
	}
	if !strings.Contains(gotQuery, "projects=default") {
		t.Errorf("expected project filter in query, got %q", gotQuery)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 application set, got %d", len(sets))
	}
	got := sets[0]
	if got.Name != "clusters-appset" || got.Namespace != "argocd" {
		t.Errorf("unexpected identity fields: %+v", got)
	}
	if len(got.Resources) != 2 || got.Resources[0] != "basra-prod" || got.Resources[1] != "basra-staging" {
		t.Errorf("unexpected generated resources: %+v", got.Resources)
	}
	if len(got.Conditions) != 1 || got.Conditions[0].Type != "ResourcesUpToDate" || got.Conditions[0].Status != "True" {
		t.Errorf("unexpected conditions: %+v", got.Conditions)
	}
}
