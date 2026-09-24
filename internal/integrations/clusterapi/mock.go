// Package clusterapi implements the ports.ClusterAPIProvider adapter
// (ADR-0002). Real manifest rendering against Talos-compatible CAPI
// infrastructure providers lands in Phase 5; this mock renders a minimal,
// well-formed manifest set so the GitOps commit pipeline can be exercised
// end-to-end for CLUSTER_API-mode clusters today.
package clusterapi

import (
	"context"
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type MockProvider struct{}

func NewMockProvider() *MockProvider { return &MockProvider{} }

func (p *MockProvider) RenderManifests(_ context.Context, c *cluster.Cluster) ([]ports.FileChange, error) {
	clusterYAML := fmt.Sprintf(`apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: %s
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks: [%q]
    services:
      cidrBlocks: [%q]
`, c.Name, c.Spec.Network.PodCIDR, c.Spec.Network.ServiceCIDR)

	mdYAML := fmt.Sprintf(`apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: %s-workers
  namespace: default
spec:
  clusterName: %s
  replicas: %d
`, c.Name, c.Name, sumWorkerReplicas(c))

	return []ports.FileChange{
		{Path: fmt.Sprintf("clusters/%s/capi-cluster.yaml", c.Name), Content: []byte(clusterYAML)},
		{Path: fmt.Sprintf("clusters/%s/capi-machinedeployment.yaml", c.Name), Content: []byte(mdYAML)},
	}, nil
}

func sumWorkerReplicas(c *cluster.Cluster) int32 {
	var total int32
	for _, w := range c.Spec.Workers {
		total += w.Replicas
	}
	return total
}

func (p *MockProvider) GetClusterStatus(_ context.Context, namespace, name string) ([]ports.CAPIResourceStatus, error) {
	return []ports.CAPIResourceStatus{
		{Kind: "Cluster", Name: name, Namespace: namespace, Phase: "Provisioned", Ready: true},
		{Kind: "MachineDeployment", Name: name + "-workers", Namespace: namespace, Phase: "Running", Ready: true},
		{Kind: "MachineHealthCheck", Name: name + "-mhc", Namespace: namespace, Phase: "healthy=0/0 remediationsAllowed=0", Ready: true},
	}, nil
}

func (p *MockProvider) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}
