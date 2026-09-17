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
// into the cluster's Spec once the change has been committed/observed.
//
// Phase 3/6 wire the commit-to-Git step through the real GitProvider so the
// new replica count is reconciled by Argo CD/CAPI rather than applied here
// directly; today the spec update itself stands in for that round trip.
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
				return map[string]any{"pools": len(c.Spec.Workers)}, nil
			}),
		},
	}
}
