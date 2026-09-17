package cluster_test

import (
	"testing"

	"github.com/talos-platform/talos-platform/internal/domain/cluster"
)

func TestState_CanTransition(t *testing.T) {
	tests := []struct {
		from, to cluster.State
		want     bool
	}{
		{cluster.StateDraft, cluster.StatePlanning, true},
		{cluster.StateDraft, cluster.StateReady, false},
		{cluster.StateReady, cluster.StateUpgrading, true},
		{cluster.StateUpgrading, cluster.StateReady, true},
		{cluster.StateDeleted, cluster.StatePlanning, false},
		{cluster.StateProvisioning, cluster.StateBootstrapping, true},
	}
	for _, tt := range tests {
		if got := tt.from.CanTransition(tt.to); got != tt.want {
			t.Errorf("State(%s).CanTransition(%s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestCluster_Transition_RejectsIllegalJump(t *testing.T) {
	c := &cluster.Cluster{State: cluster.StateDraft}
	if err := c.Transition(cluster.StateReady); err == nil {
		t.Fatal("expected error transitioning DRAFT -> READY directly")
	}
	if c.State != cluster.StateDraft {
		t.Fatalf("state should be unchanged after rejected transition, got %s", c.State)
	}
}

func TestCluster_Transition_AppliesLegalStep(t *testing.T) {
	c := &cluster.Cluster{State: cluster.StateDraft}
	if err := c.Transition(cluster.StatePlanning); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.State != cluster.StatePlanning {
		t.Fatalf("expected state PLANNING, got %s", c.State)
	}
	if c.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set after transition")
	}
}
