package clusterapi

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
)

// listKinds registers the List kind for every GVR this client queries, since
// the fake dynamic client only infers these from objects actually passed in
// — a test with no MachineDeployment/Machine objects would otherwise panic
// on List() for those resources.
var listKinds = map[schema.GroupVersionResource]string{
	clusterGVR:           "ClusterList",
	machineDeploymentGVR: "MachineDeploymentList",
	machineGVR:           "MachineList",
}

func testCluster() *cluster.Cluster {
	return &cluster.Cluster{
		Name:         "basra-prod",
		ProviderMode: cluster.ProviderModeClusterAPI,
		Spec: cluster.Spec{
			KubernetesVersion: "v1.31.1",
			TalosVersion:      "v1.8.2",
			ControlPlane:      cluster.ControlPlaneSpec{Replicas: 3},
			Workers: []cluster.WorkerPoolSpec{
				{Name: "default", Replicas: 5},
				{Name: "gpu", Replicas: 2},
			},
			Network: cluster.NetworkSpec{PodCIDR: "10.244.0.0/16", ServiceCIDR: "10.96.0.0/12"},
		},
	}
}

func TestClient_RenderManifests_ProducesExpectedFileSet(t *testing.T) {
	c := &Client{}
	files, err := c.RenderManifests(t.Context(), testCluster())
	if err != nil {
		t.Fatalf("RenderManifests: %v", err)
	}

	wantPaths := []string{
		"clusters/basra-prod/capi-cluster.yaml",
		"clusters/basra-prod/capi-control-plane.yaml",
		"clusters/basra-prod/capi-machinedeployment-default.yaml",
		"clusters/basra-prod/capi-talosconfigtemplate-default.yaml",
		"clusters/basra-prod/capi-machinedeployment-gpu.yaml",
		"clusters/basra-prod/capi-talosconfigtemplate-gpu.yaml",
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
}

func TestClient_RenderManifests_IsDeterministic(t *testing.T) {
	c := &Client{}
	cl := testCluster()
	first, err := c.RenderManifests(t.Context(), cl)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := c.RenderManifests(t.Context(), cl)
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

func unstructuredResource(apiVersion, kind, namespace, name string, status map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]any{"cluster.x-k8s.io/cluster-name": "basra-prod"},
		},
	}}
	if status != nil {
		obj.Object["status"] = status
	}
	return obj
}

func readyStatus(phase string) map[string]any {
	return map[string]any{
		"phase": phase,
		"conditions": []any{
			map[string]any{"type": "Ready", "status": "True"},
		},
	}
}

func TestClient_GetClusterStatus(t *testing.T) {
	scheme := runtime.NewScheme()

	objects := []runtime.Object{
		unstructuredResource("cluster.x-k8s.io/v1beta1", "Cluster", "default", "basra-prod", readyStatus("Provisioned")),
		unstructuredResource("cluster.x-k8s.io/v1beta1", "MachineDeployment", "default", "basra-prod-default", readyStatus("Running")),
		unstructuredResource("cluster.x-k8s.io/v1beta1", "Machine", "default", "basra-prod-default-abc12", readyStatus("Running")),
	}

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
	c := &Client{dyn: fakeClient}

	statuses, err := c.GetClusterStatus(t.Context(), "default", "basra-prod")
	if err != nil {
		t.Fatalf("GetClusterStatus: %v", err)
	}

	if len(statuses) != 3 {
		t.Fatalf("expected 3 resource statuses (Cluster + MachineDeployment + Machine), got %d: %+v", len(statuses), statuses)
	}
	for _, s := range statuses {
		if !s.Ready {
			t.Errorf("expected resource %s/%s to be Ready, got %+v", s.Kind, s.Name, s)
		}
	}
	if statuses[0].Kind != "Cluster" || statuses[0].Phase != "Provisioned" {
		t.Errorf("expected first status to be the Cluster with phase Provisioned, got %+v", statuses[0])
	}
}

func TestClient_GetClusterStatus_NotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	notReady := map[string]any{"phase": "Provisioning"} // no Ready condition yet

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
		unstructuredResource("cluster.x-k8s.io/v1beta1", "Cluster", "default", "basra-prod", notReady))
	c := &Client{dyn: fakeClient}

	statuses, err := c.GetClusterStatus(t.Context(), "default", "basra-prod")
	if err != nil {
		t.Fatalf("GetClusterStatus: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Ready {
		t.Fatalf("expected the Cluster to be reported not-ready, got %+v", statuses)
	}
}
