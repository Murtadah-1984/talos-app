// Package gitopsrender renders a cluster's desired configuration into the
// GitOps repository layout described in §3 of the product spec:
//
//	gitops/clusters/<name>/cluster.yaml
//	gitops/clusters/<name>/infrastructure.yaml
//	gitops/clusters/<name>/control-plane.yaml
//	gitops/clusters/<name>/workers.yaml
//	gitops/clusters/<name>/argocd-application.yaml   (when Argo CD is enabled)
//
// Every function here is a pure transformation from a cluster.Cluster to
// []ports.FileChange — no I/O, no side effects — so the commit step in
// internal/workflows/cluster_provision.go (and any future caller) gets
// deterministic, diffable YAML without needing to know the layout itself.
// Kustomize is deliberately not used per §3 ("prefer Helm... do not
// introduce Kustomize unless there is a compelling reason").
package gitopsrender

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
)

// Input bundles everything the renderer needs beyond the cluster's own Spec:
// where in the repository it should land, and how the resulting Argo CD
// Application (if any) should reach this Git repository.
type Input struct {
	Cluster       *cluster.Cluster
	GitOpsPath    string // e.g. "clusters/basra-prod", relative to the repo root
	RepoURL       string // e.g. "https://github.com/acme/gitops.git"
	DefaultBranch string // the branch Argo CD should track once merged
}

// clusterDocument mirrors cluster.yaml's shape (§9). Field names are chosen
// to read naturally as YAML, independent of the internal Go struct's names.
type clusterDocument struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   metadata            `yaml:"metadata"`
	Spec       clusterDocumentSpec `yaml:"spec"`
}

type metadata struct {
	Name string `yaml:"name"`
}

type clusterDocumentSpec struct {
	ProviderMode      string      `yaml:"providerMode"`
	KubernetesVersion string      `yaml:"kubernetesVersion"`
	TalosVersion      string      `yaml:"talosVersion"`
	Network           networkSpec `yaml:"network"`
}

type networkSpec struct {
	PodCIDR     string `yaml:"podCIDR"`
	ServiceCIDR string `yaml:"serviceCIDR"`
}

type infrastructureDocument struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Metadata   metadata           `yaml:"metadata"`
	Spec       infrastructureSpec `yaml:"spec"`
}

type infrastructureSpec struct {
	CNI           cniSpec           `yaml:"cni"`
	Storage       storageSpec       `yaml:"storage"`
	Ingress       ingressSpec       `yaml:"ingress"`
	Observability observabilitySpec `yaml:"observability"`
}

type cniSpec struct {
	Type string `yaml:"type"`
}

type storageSpec struct {
	CSI string `yaml:"csi"`
}

type ingressSpec struct {
	Controller string `yaml:"controller"`
}

type observabilitySpec struct {
	Prometheus    bool `yaml:"prometheus"`
	Grafana       bool `yaml:"grafana"`
	Loki          bool `yaml:"loki"`
	OpenTelemetry bool `yaml:"openTelemetry"`
}

type controlPlaneDocument struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   metadata         `yaml:"metadata"`
	Spec       controlPlaneSpec `yaml:"spec"`
}

type controlPlaneSpec struct {
	ClusterName string `yaml:"clusterName"`
	Replicas    int32  `yaml:"replicas"`
	CPU         int32  `yaml:"cpu"`
	MemoryGB    int32  `yaml:"memoryGB"`
}

type workerPoolDocument struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   metadata       `yaml:"metadata"`
	Spec       workerPoolSpec `yaml:"spec"`
}

type workerPoolSpec struct {
	ClusterName string            `yaml:"clusterName"`
	Replicas    int32             `yaml:"replicas"`
	CPU         int32             `yaml:"cpu"`
	MemoryGB    int32             `yaml:"memoryGB"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Taints      []string          `yaml:"taints,omitempty"`
}

type argoApplicationDocument struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   argoMetadata        `yaml:"metadata"`
	Spec       argoApplicationSpec `yaml:"spec"`
}

type argoMetadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type argoApplicationSpec struct {
	Project     string          `yaml:"project"`
	Source      argoSource      `yaml:"source"`
	Destination argoDestination `yaml:"destination"`
	SyncPolicy  argoSyncPolicy  `yaml:"syncPolicy"`
}

type argoSource struct {
	RepoURL        string `yaml:"repoURL"`
	Path           string `yaml:"path"`
	TargetRevision string `yaml:"targetRevision"`
}

type argoDestination struct {
	Server    string `yaml:"server"`
	Namespace string `yaml:"namespace"`
}

type argoSyncPolicy struct {
	Automated argoAutomatedSync `yaml:"automated"`
}

type argoAutomatedSync struct {
	Prune    bool `yaml:"prune"`
	SelfHeal bool `yaml:"selfHeal"`
}

// RenderClusterManifests renders the full manifest set for in.Cluster. The
// returned files are always ordered the same way for a given input, so
// successive commits produce clean, predictable diffs.
func RenderClusterManifests(in Input) ([]ports.FileChange, error) {
	c := in.Cluster
	path := strings.TrimSuffix(in.GitOpsPath, "/")

	files := make([]ports.FileChange, 0, 5)

	clusterYAML, err := marshal(clusterDocument{
		APIVersion: "platform.talos.dev/v1alpha1",
		Kind:       "Cluster",
		Metadata:   metadata{Name: c.Name},
		Spec: clusterDocumentSpec{
			ProviderMode:      string(c.ProviderMode),
			KubernetesVersion: c.Spec.KubernetesVersion,
			TalosVersion:      c.Spec.TalosVersion,
			Network: networkSpec{
				PodCIDR:     c.Spec.Network.PodCIDR,
				ServiceCIDR: c.Spec.Network.ServiceCIDR,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering cluster.yaml: %w", err)
	}
	files = append(files, ports.FileChange{Path: path + "/cluster.yaml", Content: clusterYAML})

	infraYAML, err := marshal(infrastructureDocument{
		APIVersion: "platform.talos.dev/v1alpha1",
		Kind:       "ClusterInfrastructure",
		Metadata:   metadata{Name: c.Name},
		Spec: infrastructureSpec{
			CNI:     cniSpec{Type: c.Spec.CNI.Type},
			Storage: storageSpec{CSI: c.Spec.Storage.CSI},
			Ingress: ingressSpec{Controller: c.Spec.Ingress.Controller},
			Observability: observabilitySpec{
				Prometheus:    c.Spec.Observability.Prometheus,
				Grafana:       c.Spec.Observability.Grafana,
				Loki:          c.Spec.Observability.Loki,
				OpenTelemetry: c.Spec.Observability.OpenTelemetry,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering infrastructure.yaml: %w", err)
	}
	files = append(files, ports.FileChange{Path: path + "/infrastructure.yaml", Content: infraYAML})

	cpYAML, err := marshal(controlPlaneDocument{
		APIVersion: "platform.talos.dev/v1alpha1",
		Kind:       "ControlPlane",
		Metadata:   metadata{Name: c.Name + "-control-plane"},
		Spec: controlPlaneSpec{
			ClusterName: c.Name,
			Replicas:    c.Spec.ControlPlane.Replicas,
			CPU:         c.Spec.ControlPlane.CPU,
			MemoryGB:    c.Spec.ControlPlane.MemoryGB,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering control-plane.yaml: %w", err)
	}
	files = append(files, ports.FileChange{Path: path + "/control-plane.yaml", Content: cpYAML})

	workersYAML, err := renderWorkerPools(c)
	if err != nil {
		return nil, fmt.Errorf("rendering workers.yaml: %w", err)
	}
	files = append(files, ports.FileChange{Path: path + "/workers.yaml", Content: workersYAML})

	if c.Spec.ArgoCD.Enabled {
		argoYAML, err := marshal(argoApplicationDocument{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Application",
			Metadata:   argoMetadata{Name: c.Name, Namespace: "argocd"},
			Spec: argoApplicationSpec{
				Project: c.Spec.ArgoCD.Project,
				Source: argoSource{
					RepoURL:        in.RepoURL,
					Path:           path,
					TargetRevision: in.DefaultBranch,
				},
				Destination: argoDestination{Server: "https://kubernetes.default.svc", Namespace: "default"},
				SyncPolicy:  argoSyncPolicy{Automated: argoAutomatedSync{Prune: true, SelfHeal: true}},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("rendering argocd-application.yaml: %w", err)
		}
		files = append(files, ports.FileChange{Path: path + "/argocd-application.yaml", Content: argoYAML})
	}

	return files, nil
}

// renderWorkerPools emits one YAML document per worker pool, separated by
// "---", matching how multi-document Kubernetes-style manifests are
// conventionally laid out in a single file.
func renderWorkerPools(c *cluster.Cluster) ([]byte, error) {
	if len(c.Spec.Workers) == 0 {
		return []byte("# no worker pools defined\n"), nil
	}

	var out []byte
	for i, w := range c.Spec.Workers {
		doc, err := marshal(workerPoolDocument{
			APIVersion: "platform.talos.dev/v1alpha1",
			Kind:       "WorkerPool",
			Metadata:   metadata{Name: fmt.Sprintf("%s-%s", c.Name, w.Name)},
			Spec: workerPoolSpec{
				ClusterName: c.Name,
				Replicas:    w.Replicas,
				CPU:         w.CPU,
				MemoryGB:    w.MemoryGB,
				Labels:      w.Labels,
				Taints:      w.Taints,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("rendering worker pool %q: %w", w.Name, err)
		}
		if i > 0 {
			out = append(out, []byte("---\n")...)
		}
		out = append(out, doc...)
	}
	return out, nil
}

func marshal(doc any) ([]byte, error) {
	return yaml.Marshal(doc)
}
