package gitopsrender_test

import (
	"strings"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/gitopsrender"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
)

func testCluster() *cluster.Cluster {
	return &cluster.Cluster{
		Name:         "basra-prod",
		ProviderMode: cluster.ProviderModeDirectTalos,
		Spec: cluster.Spec{
			KubernetesVersion: "v1.31.1",
			TalosVersion:      "v1.8.2",
			ControlPlane:      cluster.ControlPlaneSpec{Replicas: 3, CPU: 4, MemoryGB: 8},
			Workers: []cluster.WorkerPoolSpec{
				{Name: "default", Replicas: 5, CPU: 4, MemoryGB: 16},
				{Name: "gpu", Replicas: 2, CPU: 8, MemoryGB: 32, Labels: map[string]string{"gpu": "true"}},
			},
			Network: cluster.NetworkSpec{PodCIDR: "10.244.0.0/16", ServiceCIDR: "10.96.0.0/12"},
			CNI:     cluster.CNISpec{Type: "calico"},
			ArgoCD:  cluster.ArgoCDSpec{Enabled: true, Project: "default"},
		},
	}
}

func TestRenderClusterManifests_ProducesExpectedFileSet(t *testing.T) {
	files, err := gitopsrender.RenderClusterManifests(gitopsrender.Input{
		Cluster:       testCluster(),
		GitOpsPath:    "clusters/basra-prod",
		RepoURL:       "https://github.com/acme/gitops.git",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("RenderClusterManifests: %v", err)
	}

	wantPaths := []string{
		"clusters/basra-prod/cluster.yaml",
		"clusters/basra-prod/infrastructure.yaml",
		"clusters/basra-prod/control-plane.yaml",
		"clusters/basra-prod/workers.yaml",
		"clusters/basra-prod/argocd-application.yaml",
	}
	if len(files) != len(wantPaths) {
		t.Fatalf("expected %d files, got %d", len(wantPaths), len(files))
	}
	for i, want := range wantPaths {
		if files[i].Path != want {
			t.Errorf("file %d: expected path %q, got %q", i, want, files[i].Path)
		}
		if len(files[i].Content) == 0 {
			t.Errorf("file %d (%s): content is empty", i, files[i].Path)
		}
	}
}

func TestRenderClusterManifests_SkipsArgoCDWhenDisabled(t *testing.T) {
	c := testCluster()
	c.Spec.ArgoCD.Enabled = false

	files, err := gitopsrender.RenderClusterManifests(gitopsrender.Input{Cluster: c, GitOpsPath: "clusters/basra-prod"})
	if err != nil {
		t.Fatalf("RenderClusterManifests: %v", err)
	}
	for _, f := range files {
		if strings.Contains(f.Path, "argocd") {
			t.Errorf("expected no argocd-application.yaml when ArgoCD is disabled, got %s", f.Path)
		}
	}
}

func TestRenderClusterManifests_WorkersYAMLHasOneDocumentPerPool(t *testing.T) {
	files, err := gitopsrender.RenderClusterManifests(gitopsrender.Input{Cluster: testCluster(), GitOpsPath: "clusters/basra-prod"})
	if err != nil {
		t.Fatalf("RenderClusterManifests: %v", err)
	}

	var workersContent string
	for _, f := range files {
		if strings.HasSuffix(f.Path, "workers.yaml") {
			workersContent = string(f.Content)
		}
	}
	if workersContent == "" {
		t.Fatal("workers.yaml not found in rendered files")
	}
	if got := strings.Count(workersContent, "kind: WorkerPool"); got != 2 {
		t.Errorf("expected 2 WorkerPool documents, got %d in:\n%s", got, workersContent)
	}
	if !strings.Contains(workersContent, "---") {
		t.Error("expected worker pool documents to be separated by '---'")
	}
}

func TestRenderRepositoryScaffold_ProducesExpectedFileSet(t *testing.T) {
	files, err := gitopsrender.RenderRepositoryScaffold(gitopsrender.RepositoryScaffoldInput{
		RepoURL:       "https://github.com/acme/gitops.git",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("RenderRepositoryScaffold: %v", err)
	}

	wantPaths := []string{
		"README.md",
		"app-of-apps.yaml",
		"infrastructure/README.md",
		"applications/README.md",
	}
	if len(files) != len(wantPaths) {
		t.Fatalf("expected %d files, got %d: %+v", len(wantPaths), len(files), files)
	}
	for i, want := range wantPaths {
		if files[i].Path != want {
			t.Errorf("file %d: expected path %q, got %q", i, want, files[i].Path)
		}
		if len(files[i].Content) == 0 {
			t.Errorf("file %d (%s): content is empty", i, files[i].Path)
		}
	}

	var appOfApps string
	for _, f := range files {
		if f.Path == "app-of-apps.yaml" {
			appOfApps = string(f.Content)
		}
	}
	if !strings.Contains(appOfApps, "path: applications") {
		t.Errorf("expected app-of-apps.yaml to point at the applications/ directory, got:\n%s", appOfApps)
	}
	if !strings.Contains(appOfApps, "repoURL: https://github.com/acme/gitops.git") {
		t.Errorf("expected app-of-apps.yaml to reference the repo URL, got:\n%s", appOfApps)
	}
}

func TestRenderRepositoryScaffold_IsDeterministic(t *testing.T) {
	in := gitopsrender.RepositoryScaffoldInput{RepoURL: "https://github.com/acme/gitops.git", DefaultBranch: "main"}
	first, err := gitopsrender.RenderRepositoryScaffold(in)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := gitopsrender.RenderRepositoryScaffold(in)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("file count differs between renders: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if string(first[i].Content) != string(second[i].Content) {
			t.Errorf("file %s rendered differently on repeated calls", first[i].Path)
		}
	}
}

func TestRenderClusterManifests_IsDeterministic(t *testing.T) {
	in := gitopsrender.Input{Cluster: testCluster(), GitOpsPath: "clusters/basra-prod"}
	first, err := gitopsrender.RenderClusterManifests(in)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := gitopsrender.RenderClusterManifests(in)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("file count differs between renders: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if string(first[i].Content) != string(second[i].Content) {
			t.Errorf("file %s rendered differently on repeated calls", first[i].Path)
		}
	}
}
