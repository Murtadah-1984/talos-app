package workflows

import (
	"context"
	"fmt"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
)

// NewWorkerScaleDefinition builds the WORKER_SCALE workflow (§17 "Scale
// Workers"). The desired replica count for the named pool is read from the
// workflow's Input (set by clusterservice.ScaleWorkers) and written back
// into the cluster's Spec (by the caller) before this workflow runs.
//
// DIRECT_TALOS-mode clusters apply the new replica count directly (Phase 6
// wires this to the infrastructure provider's ProvisionMachine/DeleteMachine
// once bare-metal/Proxmox are real — today it's a placeholder). CLUSTER_API-
// mode clusters commit the new replica count into their MachineDeployment
// manifest and let CAPI controllers actually scale (ADR-0002).
func NewWorkerScaleDefinition(deps ClusterProvisionDeps) Definition {
	step := func(name string, fn func(context.Context, *cluster.Cluster) (map[string]any, error)) StepDefinition {
		return StepDefinition{Name: name, Run: clusterStep(deps.Clusters, fn)}
	}

	return Definition{
		Type: workflow.TypeWorkerScale,
		Steps: []StepDefinition{
			step("validate-scale-request", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if c.State != cluster.StateReady && c.State != cluster.StateDegraded {
					return nil, fmt.Errorf("cluster must be READY or DEGRADED to scale, was %s", c.State)
				}
				return map[string]any{"currentState": string(c.State)}, nil
			}),
			step("apply-scale", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if c.ProviderMode == cluster.ProviderModeClusterAPI {
					return map[string]any{"skipped": true, "reason": "CAPI-managed"}, nil
				}
				return map[string]any{"pools": len(c.Spec.Workers)}, nil
			}),
			step("commit-capi-scale", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if c.ProviderMode != cluster.ProviderModeClusterAPI {
					return map[string]any{"skipped": true}, nil
				}
				desc := fmt.Sprintf("Scale cluster %s worker pools", c.Name)
				if err := commitClusterAPIChange(ctx, deps, c, "scale", desc); err != nil {
					return nil, err
				}
				return map[string]any{"committed": true}, nil
			}),
			step("wait-for-cluster-api", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if err := waitForClusterAPIReady(ctx, deps, c); err != nil {
					return nil, err
				}
				return map[string]any{"capiReady": c.ProviderMode == cluster.ProviderModeClusterAPI}, nil
			}),
		},
	}
}
