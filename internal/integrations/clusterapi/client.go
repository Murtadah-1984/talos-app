// Package clusterapi implements the ports.ClusterAPIProvider adapter
// (ADR-0002). This file is the Phase 5 real implementation:
//
//   - RenderManifests is a pure function (no I/O) producing real Cluster API
//     resources for a Talos-based cluster: the core `Cluster` and
//     `MachineDeployment` resources (cluster.x-k8s.io/v1beta1, well-established
//     upstream schema), plus `TalosControlPlane`/`TalosConfigTemplate`
//     (from Sidero Labs' Cluster API Control Plane/Bootstrap Providers for
//     Talos) rendered best-effort — those two CRDs are less universally
//     documented than core CAPI, so treat their exact field names as a
//     starting point to verify against the CACPPT/CABPT version actually
//     installed in your management cluster, not a guarantee.
//   - GetClusterStatus is real, read-only observation against a CAPI
//     management cluster's Kubernetes API (ADR-0002: "the platform must
//     NOT duplicate Cluster API's reconciliation logic" — this client never
//     writes Cluster API resources directly; manifests are written to Git,
//     never applied here).
//
// Provider-specific infrastructure CRs (the `infrastructureRef` a real
// deployment would point at — e.g. a bare-metal or Proxmox infra provider's
// own Cluster API CRDs) are not rendered here: no infrastructure provider in
// this codebase is CAPI-aware yet (Phase 6 is still mock-only), so
// fabricating a specific infrastructure CRD shape would be more misleading
// than useful. See docs/cluster-api/README.md.
package clusterapi

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Client is the real ports.ClusterAPIProvider adapter.
type Client struct {
	dyn dynamic.Interface
}

// NewClient builds a Client from a kubeconfig pointing at a Cluster API
// management cluster.
func NewClient(kubeconfigPath string) (*Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading management cluster kubeconfig: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building dynamic client: %w", err)
	}
	return &Client{dyn: dyn}, nil
}

var (
	clusterGVR           = schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "clusters"}
	machineDeploymentGVR = schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machinedeployments"}
	machineGVR           = schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machines"}
)

// GetClusterStatus reads the Cluster resource and every MachineDeployment/
// Machine labeled for it (the standard `cluster.x-k8s.io/cluster-name`
// label CAPI itself applies) from the management cluster, read-only.
func (c *Client) GetClusterStatus(ctx context.Context, namespace, name string) ([]ports.CAPIResourceStatus, error) {
	var out []ports.CAPIResourceStatus

	clusterObj, err := c.dyn.Resource(clusterGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting Cluster %s/%s: %w", namespace, name, err)
	}
	out = append(out, resourceStatus("Cluster", clusterObj))

	selector := "cluster.x-k8s.io/cluster-name=" + name

	mds, err := c.dyn.Resource(machineDeploymentGVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("listing MachineDeployments for cluster %s: %w", name, err)
	}
	for i := range mds.Items {
		out = append(out, resourceStatus("MachineDeployment", &mds.Items[i]))
	}

	machines, err := c.dyn.Resource(machineGVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("listing Machines for cluster %s: %w", name, err)
	}
	for i := range machines.Items {
		out = append(out, resourceStatus("Machine", &machines.Items[i]))
	}

	return out, nil
}

// resourceStatus maps any CAPI resource's observed state into
// ports.CAPIResourceStatus using the conventions shared across the whole
// CAPI resource family: a `status.phase` string where present, and a
// `status.conditions[]` entry of type "Ready" for readiness.
func resourceStatus(kind string, obj *unstructured.Unstructured) ports.CAPIResourceStatus {
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	return ports.CAPIResourceStatus{
		Kind:      kind,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Phase:     phase,
		Ready:     readyCondition(obj),
	}
}

func readyCondition(obj *unstructured.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, raw := range conditions {
		cond, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")
		if condType == "Ready" && condStatus == "True" {
			return true
		}
	}
	return false
}

func (c *Client) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}

// --- RenderManifests: pure manifest generation, no client required ---

type typeMeta struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

type objectMeta struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

type objectRef struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Name       string `yaml:"name"`
	Namespace  string `yaml:"namespace,omitempty"`
}

// capiCluster mirrors cluster.x-k8s.io/v1beta1 Cluster — core CAPI, stable.
type capiCluster struct {
	typeMeta `yaml:",inline"`
	Metadata objectMeta      `yaml:"metadata"`
	Spec     capiClusterSpec `yaml:"spec"`
}

type capiClusterSpec struct {
	ClusterNetwork  clusterNetwork `yaml:"clusterNetwork"`
	ControlPlaneRef objectRef      `yaml:"controlPlaneRef"`
}

type clusterNetwork struct {
	Pods     cidrBlocks `yaml:"pods"`
	Services cidrBlocks `yaml:"services"`
}

type cidrBlocks struct {
	CIDRBlocks []string `yaml:"cidrBlocks"`
}

// talosControlPlane mirrors controlplane.cluster.x-k8s.io/v1alpha3
// TalosControlPlane from Sidero Labs' Cluster API Control Plane Provider for
// Talos — best-effort, see the package doc comment.
type talosControlPlane struct {
	typeMeta `yaml:",inline"`
	Metadata objectMeta            `yaml:"metadata"`
	Spec     talosControlPlaneSpec `yaml:"spec"`
}

type talosControlPlaneSpec struct {
	Replicas               int32     `yaml:"replicas"`
	Version                string    `yaml:"version"`
	InfrastructureTemplate objectRef `yaml:"infrastructureTemplate"`
}

// machineDeployment mirrors cluster.x-k8s.io/v1beta1 MachineDeployment —
// core CAPI, stable.
type machineDeployment struct {
	typeMeta `yaml:",inline"`
	Metadata objectMeta            `yaml:"metadata"`
	Spec     machineDeploymentSpec `yaml:"spec"`
}

type machineDeploymentSpec struct {
	ClusterName string          `yaml:"clusterName"`
	Replicas    int32           `yaml:"replicas"`
	Selector    labelSelector   `yaml:"selector"`
	Template    machineTemplate `yaml:"template"`
}

type labelSelector struct {
	MatchLabels map[string]string `yaml:"matchLabels"`
}

type machineTemplate struct {
	Metadata objectMeta          `yaml:"metadata"`
	Spec     machineTemplateSpec `yaml:"spec"`
}

type machineTemplateSpec struct {
	ClusterName string    `yaml:"clusterName"`
	Version     string    `yaml:"version"`
	Bootstrap   bootstrap `yaml:"bootstrap"`
}

type bootstrap struct {
	ConfigRef objectRef `yaml:"configRef"`
}

// talosConfigTemplate mirrors bootstrap.cluster.x-k8s.io/v1alpha3
// TalosConfigTemplate from Sidero Labs' Cluster API Bootstrap Provider for
// Talos — best-effort, see the package doc comment.
type talosConfigTemplate struct {
	typeMeta `yaml:",inline"`
	Metadata objectMeta              `yaml:"metadata"`
	Spec     talosConfigTemplateSpec `yaml:"spec"`
}

type talosConfigTemplateSpec struct {
	Template talosConfigTemplateInner `yaml:"template"`
}

type talosConfigTemplateInner struct {
	Spec talosConfigSpec `yaml:"spec"`
}

type talosConfigSpec struct {
	GenerateType string `yaml:"generateType"`
	TalosVersion string `yaml:"talosVersion"`
}

const (
	capiAPIVersion       = "cluster.x-k8s.io/v1beta1"
	cacpptAPIVersion     = "controlplane.cluster.x-k8s.io/v1alpha3"
	cabptAPIVersion      = "bootstrap.cluster.x-k8s.io/v1alpha3"
	defaultCAPINamespace = "default"
)

// RenderManifests renders the Cluster API resource set for c: Cluster,
// TalosControlPlane, and one MachineDeployment + TalosConfigTemplate pair
// per worker pool. Deterministic and side-effect-free, matching
// gitopsrender's contract, so the commit-to-git workflow step can call it
// repeatedly without producing spurious diffs.
func (c *Client) RenderManifests(_ context.Context, cl *cluster.Cluster) ([]ports.FileChange, error) {
	ns := defaultCAPINamespace
	base := fmt.Sprintf("clusters/%s", cl.Name)

	var files []ports.FileChange

	controlPlaneRef := objectRef{
		APIVersion: cacpptAPIVersion,
		Kind:       "TalosControlPlane",
		Name:       cl.Name + "-control-plane",
		Namespace:  ns,
	}

	clusterYAML, err := marshalCAPI(capiCluster{
		typeMeta: typeMeta{APIVersion: capiAPIVersion, Kind: "Cluster"},
		Metadata: objectMeta{Name: cl.Name, Namespace: ns},
		Spec: capiClusterSpec{
			ClusterNetwork: clusterNetwork{
				Pods:     cidrBlocks{CIDRBlocks: []string{cl.Spec.Network.PodCIDR}},
				Services: cidrBlocks{CIDRBlocks: []string{cl.Spec.Network.ServiceCIDR}},
			},
			ControlPlaneRef: controlPlaneRef,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering Cluster: %w", err)
	}
	files = append(files, ports.FileChange{Path: base + "/capi-cluster.yaml", Content: clusterYAML})

	cpYAML, err := marshalCAPI(talosControlPlane{
		typeMeta: typeMeta{APIVersion: cacpptAPIVersion, Kind: "TalosControlPlane"},
		Metadata: objectMeta{Name: controlPlaneRef.Name, Namespace: ns},
		Spec: talosControlPlaneSpec{
			Replicas: cl.Spec.ControlPlane.Replicas,
			Version:  cl.Spec.KubernetesVersion,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering TalosControlPlane: %w", err)
	}
	files = append(files, ports.FileChange{Path: base + "/capi-control-plane.yaml", Content: cpYAML})

	for _, pool := range cl.Spec.Workers {
		mdName := fmt.Sprintf("%s-%s", cl.Name, pool.Name)
		configRef := objectRef{APIVersion: cabptAPIVersion, Kind: "TalosConfigTemplate", Name: mdName, Namespace: ns}

		mdYAML, err := marshalCAPI(machineDeployment{
			typeMeta: typeMeta{APIVersion: capiAPIVersion, Kind: "MachineDeployment"},
			Metadata: objectMeta{Name: mdName, Namespace: ns, Labels: map[string]string{"cluster.x-k8s.io/cluster-name": cl.Name}},
			Spec: machineDeploymentSpec{
				ClusterName: cl.Name,
				Replicas:    pool.Replicas,
				Selector:    labelSelector{MatchLabels: map[string]string{"cluster.x-k8s.io/cluster-name": cl.Name, "pool": pool.Name}},
				Template: machineTemplate{
					Metadata: objectMeta{Name: mdName, Labels: map[string]string{"cluster.x-k8s.io/cluster-name": cl.Name, "pool": pool.Name}},
					Spec: machineTemplateSpec{
						ClusterName: cl.Name,
						Version:     cl.Spec.KubernetesVersion,
						Bootstrap:   bootstrap{ConfigRef: configRef},
					},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("rendering MachineDeployment for pool %q: %w", pool.Name, err)
		}
		files = append(files, ports.FileChange{Path: fmt.Sprintf("%s/capi-machinedeployment-%s.yaml", base, pool.Name), Content: mdYAML})

		tcYAML, err := marshalCAPI(talosConfigTemplate{
			typeMeta: typeMeta{APIVersion: cabptAPIVersion, Kind: "TalosConfigTemplate"},
			Metadata: objectMeta{Name: mdName, Namespace: ns},
			Spec: talosConfigTemplateSpec{
				Template: talosConfigTemplateInner{
					Spec: talosConfigSpec{GenerateType: "worker", TalosVersion: cl.Spec.TalosVersion},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("rendering TalosConfigTemplate for pool %q: %w", pool.Name, err)
		}
		files = append(files, ports.FileChange{Path: fmt.Sprintf("%s/capi-talosconfigtemplate-%s.yaml", base, pool.Name), Content: tcYAML})
	}

	return files, nil
}

func marshalCAPI(doc any) ([]byte, error) {
	return yaml.Marshal(doc)
}

var _ ports.ClusterAPIProvider = (*Client)(nil)
