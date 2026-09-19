package leaderelection

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/talos-platform/talos-platform/internal/infrastructure/redis"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestElector_SingleReplicaBecomesLeader(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.New(mr.Addr())
	e := New(client, "scheduler-leader", time.Minute, discardLogger())

	if !e.IsLeader(t.Context()) {
		t.Fatal("expected the sole replica to become leader")
	}
	// A second call renews rather than re-acquiring; still leader.
	if !e.IsLeader(t.Context()) {
		t.Fatal("expected the leader to remain leader on renewal")
	}
}

func TestElector_SecondReplicaIsStandby(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.New(mr.Addr())

	leader := New(client, "scheduler-leader", time.Minute, discardLogger())
	standby := New(client, "scheduler-leader", time.Minute, discardLogger())

	if !leader.IsLeader(t.Context()) {
		t.Fatal("expected the first elector to become leader")
	}
	if standby.IsLeader(t.Context()) {
		t.Fatal("expected the second elector to remain standby while the first holds the lease")
	}
}

func TestElector_StandbyTakesOverAfterLeaderLeaseExpires(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.New(mr.Addr())

	leader := New(client, "scheduler-leader", 20*time.Millisecond, discardLogger())
	standby := New(client, "scheduler-leader", time.Minute, discardLogger())

	if !leader.IsLeader(t.Context()) {
		t.Fatal("expected the first elector to become leader")
	}
	mr.FastForward(30 * time.Millisecond) // past the leader's TTL; it never renews again in this test

	if !standby.IsLeader(t.Context()) {
		t.Fatal("expected the standby to take over once the leader's lease expired")
	}
}

func TestElector_ResignReleasesLeaseImmediately(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.New(mr.Addr())

	leader := New(client, "scheduler-leader", time.Minute, discardLogger())
	standby := New(client, "scheduler-leader", time.Minute, discardLogger())

	if !leader.IsLeader(t.Context()) {
		t.Fatal("expected the first elector to become leader")
	}
	leader.Resign(t.Context())

	if !standby.IsLeader(t.Context()) {
		t.Fatal("expected the standby to take over immediately after Resign, without waiting for TTL")
	}
}
