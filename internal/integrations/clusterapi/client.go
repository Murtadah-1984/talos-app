// Package clusterapi implements the ports.ClusterAPIProvider adapter
// (ADR-0002). This file is the Phase 5 real implementation:
//
//   - RenderManifests is a pure function (no I/O) producing real Cluster API
//     resources for a Talos-based cluster: the core `Cluster` and
//     `MachineDeployment` resources (cluster.x-k8s.io/v1beta1, well-established
//     upstream schema), plus `TalosControlPlane`/`TalosConfigTemplate`
//     (from Sidero Labs' Cluster API Control Plane/Bootstrap Providers for
//     Talos) and, when Proxmox infrastructure is configured (Phase 8),
//     `ProxmoxCluster`/`ProxmoxMachineTemplate` (from
//     github.com/ionos-cloud/cluster-api-provider-proxmox, "CAPMOX") wired
//     into `infrastructureRef`/`infrastructureTemplate` — all rendered
//     best-effort. These CRDs are less universally documented/stable than
//     core CAPI, so treat their exact field names as a starting point to
//     verify against the CACPPT/CABPT/CAPMOX version actually installed in
//     your management cluster, not a guarantee.
//   - GetClusterStatus is real, read-only observation against a CAPI
//     management cluster's Kubernetes API (ADR-0002: "the platform must
//     NOT duplicate Cluster API's reconciliation logic" — this client never
//     writes Cluster API resources directly; manifests are written to Git,
//     never applied here).
//
// Only Proxmox is CAPI-aware today (bare metal has no comparably established
// CAPI infrastructure provider to target) — see docs/cluster-api/README.md.
package clusterapi

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

// ProxmoxInfrastructure configures the CAPMOX-provider CRs RenderManifests
// renders for a CLUSTER_API-mode cluster. The zero value means "not
// configured": RenderManifests then renders core CAPI resources only, with
// no infrastructureRef, exactly as before this existed — existing
// DIRECT_TALOS and non-Proxmox CLUSTER_API callers are unaffected.
//
// Sourced from the same global configuration the real Proxmox
// InfrastructureProvider client already uses (internal/integrations/proxmox)
// rather than a per-organization infraprovider.InfrastructureProvider
// record — the same "Phase 2-6 bootstrapping simplification, one process-wide
// instance from global config" pattern documented in
// docs/infrastructure/README.md, not a new inconsistency.
type ProxmoxInfrastructure struct {
	// Endpoint is the Proxmox API URL (e.g. "https://pve.example.com:8006").
	// Empty means "not configured."
	Endpoint string
	// Node is the Proxmox node new VMs are created on.
	Node string
	// TemplateVMID is the pre-built Talos VM template CAPMOX clones.
	TemplateVMID int
	// CredentialSecretName is the name of a Kubernetes Secret, already
	// present in the management cluster's default namespace, holding
	// Proxmox API credentials in the shape CAPMOX's ProxmoxCluster
	// credentialsRef expects. The platform does not create this Secret —
	// unlike GitOps-committed manifests, a live credential Secret in the
	// management cluster is provisioned out of band by the operator, the
	// same way every other CAPI infrastructure provider's credentials are.
	// Defaults to "<cluster-name>-proxmox-credentials" when empty.
	CredentialSecretName string
}

func (p ProxmoxInfrastructure) configured() bool { return p.Endpoint != "" }

func (p ProxmoxInfrastructure) credentialSecretName(clusterName string) string {
	if p.CredentialSecretName != "" {
		return p.CredentialSecretName
	}
	return clusterName + "-proxmox-credentials"
}

// Client is the real ports.ClusterAPIProvider adapter.
type Client struct {
	dyn     dynamic.Interface
	proxmox ProxmoxInfrastructure
}

// NewClient builds a Client from a kubeconfig pointing at a Cluster API
// management cluster. proxmox is optional (zero value disables Proxmox
// infrastructure CR rendering).
func NewClient(kubeconfigPath string, proxmox ProxmoxInfrastructure) (*Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading management cluster kubeconfig: %w", err)
	}
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building dynamic client: %w", err)
	}
	return &Client{dyn: dyn, proxmox: proxmox}, nil
}

var (
	clusterGVR            = schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "clusters"}
	machineDeploymentGVR  = schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machinedeployments"}
	machineGVR            = schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machines"}
	machineHealthCheckGVR = schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machinehealthchecks"}
)

// GetClusterStatus reads the Cluster resource and every MachineDeployment/
// Machine/MachineHealthCheck labeled for it (the standard
// `cluster.x-k8s.io/cluster-name` label CAPI itself applies) from the
// management cluster, read-only. MachineHealthCheck is included so an
// operator can see remediation CAPI's own MachineHealthCheck controller is
// performing (replacing unhealthy Machines) without this platform ever
// initiating or duplicating that remediation itself (ADR-0002).
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

	mhcs, err := c.dyn.Resource(machineHealthCheckGVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("listing MachineHealthChecks for cluster %s: %w", name, err)
	}
	for i := range mhcs.Items {
		out = append(out, machineHealthCheckStatus(&mhcs.Items[i]))
	}

	return out, nil
}

// machineHealthCheckStatus maps a MachineHealthCheck's observed state.
// Unlike other CAPI resources, MHC doesn't use a `status.phase` string or a
// "Ready" condition — its own status fields (`currentHealthy`,
// `expectedMachines`, `remediationsAllowed`) describe whether it is
// currently allowed to remediate, which this maps onto the shared
// CAPIResourceStatus.Ready/Phase shape rather than adding a bespoke type.
func machineHealthCheckStatus(obj *unstructured.Unstructured) ports.CAPIResourceStatus {
	currentHealthy, _, _ := unstructured.NestedInt64(obj.Object, "status", "currentHealthy")
	expectedMachines, _, _ := unstructured.NestedInt64(obj.Object, "status", "expectedMachines")
	remediationsAllowed, _, _ := unstructured.NestedInt64(obj.Object, "status", "remediationsAllowed")
	return ports.CAPIResourceStatus{
		Kind:      "MachineHealthCheck",
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Phase:     fmt.Sprintf("healthy=%d/%d remediationsAllowed=%d", currentHealthy, expectedMachines, remediationsAllowed),
		Ready:     currentHealthy == expectedMachines,
	}
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
	// InfrastructureRef points at this cluster's infrastructure provider CR
	// (e.g. ProxmoxCluster). Omitted (zero value) when no infrastructure
	// provider is CAPI-aware for this cluster.
	InfrastructureRef *objectRef `yaml:"infrastructureRef,omitempty"`
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
	// InfrastructureRef points at this machine's infrastructure provider
	// template CR (e.g. ProxmoxMachineTemplate). Omitted (zero value) when
	// no infrastructure provider is CAPI-aware for this cluster.
	InfrastructureRef *objectRef `yaml:"infrastructureRef,omitempty"`
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

// proxmoxCluster mirrors infrastructure.cluster.x-k8s.io/v1alpha1
// ProxmoxCluster from github.com/ionos-cloud/cluster-api-provider-proxmox
// ("CAPMOX") — best-effort, see the package doc comment.
type proxmoxCluster struct {
	typeMeta `yaml:",inline"`
	Metadata objectMeta         `yaml:"metadata"`
	Spec     proxmoxClusterSpec `yaml:"spec"`
}

type proxmoxClusterSpec struct {
	ControlPlaneEndpoint controlPlaneEndpoint `yaml:"controlPlaneEndpoint"`
	AllowedNodes         []string             `yaml:"allowedNodes"`
	CredentialsRef       credentialsRef       `yaml:"credentialsRef"`
}

type controlPlaneEndpoint struct {
	Host string `yaml:"host"`
	Port int32  `yaml:"port"`
}

type credentialsRef struct {
	Name string `yaml:"name"`
}

// proxmoxMachineTemplate mirrors infrastructure.cluster.x-k8s.io/v1alpha1
// ProxmoxMachineTemplate ("CAPMOX") — best-effort, see the package doc
// comment.
type proxmoxMachineTemplate struct {
	typeMeta `yaml:",inline"`
	Metadata objectMeta                 `yaml:"metadata"`
	Spec     proxmoxMachineTemplateSpec `yaml:"spec"`
}

type proxmoxMachineTemplateSpec struct {
	Template proxmoxMachineTemplateInner `yaml:"template"`
}

type proxmoxMachineTemplateInner struct {
	Spec proxmoxMachineSpec `yaml:"spec"`
}

type proxmoxMachineSpec struct {
	SourceNode string `yaml:"sourceNode"`
	TemplateID int    `yaml:"templateID"`
}

const (
	capiAPIVersion       = "cluster.x-k8s.io/v1beta1"
	cacpptAPIVersion     = "controlplane.cluster.x-k8s.io/v1alpha3"
	cabptAPIVersion      = "bootstrap.cluster.x-k8s.io/v1alpha3"
	capmoxAPIVersion     = "infrastructure.cluster.x-k8s.io/v1alpha1"
	defaultCAPINamespace = "default"
	// defaultKubeAPIPort is the standard kube-apiserver port, used as the
	// ProxmoxCluster controlPlaneEndpoint's port — CAPMOX doesn't infer this,
	// and this codebase has no per-cluster override for it yet.
	defaultKubeAPIPort = 6443
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

	var clusterInfraRef *objectRef
	if c.proxmox.configured() {
		proxmoxClusterRef := objectRef{APIVersion: capmoxAPIVersion, Kind: "ProxmoxCluster", Name: cl.Name, Namespace: ns}
		clusterInfraRef = &proxmoxClusterRef

		pcYAML, err := marshalCAPI(proxmoxCluster{
			typeMeta: typeMeta{APIVersion: capmoxAPIVersion, Kind: "ProxmoxCluster"},
			Metadata: objectMeta{Name: cl.Name, Namespace: ns},
			Spec: proxmoxClusterSpec{
				ControlPlaneEndpoint: controlPlaneEndpoint{Host: cl.Endpoint, Port: defaultKubeAPIPort},
				AllowedNodes:         []string{c.proxmox.Node},
				CredentialsRef:       credentialsRef{Name: c.proxmox.credentialSecretName(cl.Name)},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("rendering ProxmoxCluster: %w", err)
		}
		files = append(files, ports.FileChange{Path: base + "/capi-proxmox-cluster.yaml", Content: pcYAML})
	}

	clusterYAML, err := marshalCAPI(capiCluster{
		typeMeta: typeMeta{APIVersion: capiAPIVersion, Kind: "Cluster"},
		Metadata: objectMeta{Name: cl.Name, Namespace: ns},
		Spec: capiClusterSpec{
			ClusterNetwork: clusterNetwork{
				Pods:     cidrBlocks{CIDRBlocks: []string{cl.Spec.Network.PodCIDR}},
				Services: cidrBlocks{CIDRBlocks: []string{cl.Spec.Network.ServiceCIDR}},
			},
			ControlPlaneRef:   controlPlaneRef,
			InfrastructureRef: clusterInfraRef,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering Cluster: %w", err)
	}
	files = append(files, ports.FileChange{Path: base + "/capi-cluster.yaml", Content: clusterYAML})

	var cpInfraTemplate objectRef
	if c.proxmox.configured() {
		cpInfraTemplate = objectRef{APIVersion: capmoxAPIVersion, Kind: "ProxmoxMachineTemplate", Name: controlPlaneRef.Name, Namespace: ns}
		pmtYAML, err := marshalCAPI(proxmoxMachineTemplate{
			typeMeta: typeMeta{APIVersion: capmoxAPIVersion, Kind: "ProxmoxMachineTemplate"},
			Metadata: objectMeta{Name: controlPlaneRef.Name, Namespace: ns},
			Spec: proxmoxMachineTemplateSpec{
				Template: proxmoxMachineTemplateInner{
					Spec: proxmoxMachineSpec{SourceNode: c.proxmox.Node, TemplateID: c.proxmox.TemplateVMID},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("rendering ProxmoxMachineTemplate for control plane: %w", err)
		}
		files = append(files, ports.FileChange{Path: base + "/capi-proxmox-machinetemplate-control-plane.yaml", Content: pmtYAML})
	}

	cpYAML, err := marshalCAPI(talosControlPlane{
		typeMeta: typeMeta{APIVersion: cacpptAPIVersion, Kind: "TalosControlPlane"},
		Metadata: objectMeta{Name: controlPlaneRef.Name, Namespace: ns},
		Spec: talosControlPlaneSpec{
			Replicas:               cl.Spec.ControlPlane.Replicas,
			Version:                cl.Spec.KubernetesVersion,
			InfrastructureTemplate: cpInfraTemplate,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering TalosControlPlane: %w", err)
	}
	files = append(files, ports.FileChange{Path: base + "/capi-control-plane.yaml", Content: cpYAML})

	for _, pool := range cl.Spec.Workers {
		mdName := fmt.Sprintf("%s-%s", cl.Name, pool.Name)
		configRef := objectRef{APIVersion: cabptAPIVersion, Kind: "TalosConfigTemplate", Name: mdName, Namespace: ns}

		var workerInfraRef *objectRef
		if c.proxmox.configured() {
			proxmoxMTRef := objectRef{APIVersion: capmoxAPIVersion, Kind: "ProxmoxMachineTemplate", Name: mdName, Namespace: ns}
			workerInfraRef = &proxmoxMTRef

			pmtYAML, err := marshalCAPI(proxmoxMachineTemplate{
				typeMeta: typeMeta{APIVersion: capmoxAPIVersion, Kind: "ProxmoxMachineTemplate"},
				Metadata: objectMeta{Name: mdName, Namespace: ns},
				Spec: proxmoxMachineTemplateSpec{
					Template: proxmoxMachineTemplateInner{
						Spec: proxmoxMachineSpec{SourceNode: c.proxmox.Node, TemplateID: c.proxmox.TemplateVMID},
					},
				},
			})
			if err != nil {
				return nil, fmt.Errorf("rendering ProxmoxMachineTemplate for pool %q: %w", pool.Name, err)
			}
			files = append(files, ports.FileChange{Path: fmt.Sprintf("%s/capi-proxmox-machinetemplate-%s.yaml", base, pool.Name), Content: pmtYAML})
		}

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
						ClusterName:       cl.Name,
						Version:           cl.Spec.KubernetesVersion,
						Bootstrap:         bootstrap{ConfigRef: configRef},
						InfrastructureRef: workerInfraRef,
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
