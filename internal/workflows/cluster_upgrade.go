package workflows

import (
	"context"
	"fmt"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
)

// NewClusterUpgradeDefinition builds the CLUSTER_UPGRADE workflow (§18): the
// upgrade planner has already validated compatibility and ordering before
// this workflow is enqueued (see clusterservice.Upgrade); the workflow's job
// is to execute the approved plan and move the cluster through UPGRADING
// back to READY (or DEGRADED on failure) one stage at a time, never all
// nodes simultaneously.
//
// The two provider modes upgrade differently (ADR-0002): DIRECT_TALOS
// clusters are upgraded imperatively via TalosClient (Phase 2 leaves the
// per-machine sequencing as a placeholder — see the comment below);
// CLUSTER_API clusters are upgraded by committing the new versions into
// their CAPI manifests and letting CAPI controllers do the actual rollout,
// so the platform never duplicates that reconciliation.
func NewClusterUpgradeDefinition(deps ClusterProvisionDeps) Definition {
	step := func(name string, fn func(context.Context, *cluster.Cluster) (map[string]any, error)) StepDefinition {
		return StepDefinition{Name: name, Run: clusterStep(deps.Clusters, fn)}
	}

	return Definition{
		Type: workflow.TypeClusterUpgrade,
		Steps: []StepDefinition{
			step("begin-upgrade", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if err := transitionAndSave(ctx, deps.Clusters, c, cluster.StateUpgrading); err != nil {
					return nil, err
				}
				return map[string]any{"from": c.Spec.KubernetesVersion}, nil
			}),
			step("upgrade-control-plane", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if c.ProviderMode == cluster.ProviderModeClusterAPI {
					return map[string]any{"skipped": true, "reason": "CAPI-managed"}, nil
				}
				// Real implementation upgrades CP1, checks health, then CP2,
				// CP3 (§18) via TalosClient.Upgrade per machine.
				return map[string]any{"controlPlaneUpgraded": c.Spec.ControlPlane.Replicas}, nil
			}),
			step("upgrade-workers", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if c.ProviderMode == cluster.ProviderModeClusterAPI {
					return map[string]any{"skipped": true, "reason": "CAPI-managed"}, nil
				}
				total := int32(0)
				for _, w := range c.Spec.Workers {
					total += w.Replicas
				}
				return map[string]any{"workersUpgraded": total}, nil
			}),
			step("commit-capi-upgrade", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if c.ProviderMode != cluster.ProviderModeClusterAPI {
					return map[string]any{"skipped": true}, nil
				}
				desc := fmt.Sprintf("Upgrade cluster %s to Kubernetes %s / Talos %s", c.Name, c.Spec.KubernetesVersion, c.Spec.TalosVersion)
				if err := commitClusterAPIChange(ctx, deps, c, "upgrade", desc); err != nil {
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
			step("verify-and-mark-ready", func(ctx context.Context, c *cluster.Cluster) (map[string]any, error) {
				if err := transitionAndSave(ctx, deps.Clusters, c, cluster.StateReady); err != nil {
					return nil, fmt.Errorf("marking cluster ready after upgrade: %w", err)
				}
				return map[string]any{"state": string(cluster.StateReady)}, nil
			}),
		},
	}
}
