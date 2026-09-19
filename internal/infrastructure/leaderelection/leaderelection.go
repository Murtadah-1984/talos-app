// Package leaderelection provides a simple, Redis-lease-backed leader
// election primitive (§22, Phase 7 HA) for components that run periodic
// background work that must not execute concurrently across replicas —
// platform-scheduler's reconciliation loop is the motivating case
// (previously documented as "never scale beyond 1 without adding leader
// election," but nothing implemented that).
//
// This deliberately does not use Postgres advisory locks: those need a
// single long-held connection, which doesn't fit a pooled pgxpool.Pool
// cleanly, whereas a Redis lease with periodic renewal (already built for
// ADR-0005's short-lived coordination needs) is a natural fit and gives
// automatic failover — a crashed leader's lease simply expires.
package leaderelection

import (
	"context"
	"log/slog"
	"time"

	"github.com/talos-platform/talos-platform/internal/infrastructure/redis"
)

// Elector tracks whether this process currently holds a named leadership
// lease. It is not safe for concurrent use from multiple goroutines — call
// IsLeader from the same loop that does the leader-only work.
type Elector struct {
	client *redis.Client
	key    string
	ttl    time.Duration
	logger *slog.Logger

	lock *redis.Lock
}

// New builds an Elector for the given lease key. ttl should comfortably
// exceed the interval between IsLeader calls (the caller's tick period) so
// a slow tick doesn't cause a spurious hand-off.
func New(client *redis.Client, key string, ttl time.Duration, logger *slog.Logger) *Elector {
	return &Elector{client: client, key: key, ttl: ttl, logger: logger}
}

// IsLeader attempts to renew this process's existing lease, or acquire a
// fresh one if it doesn't hold one, and reports whether it is the leader as
// of this call. Intended to be called once per tick of the caller's loop,
// gating whether that tick's work runs.
func (e *Elector) IsLeader(ctx context.Context) bool {
	if e.lock != nil {
		renewed, err := e.lock.Renew(ctx, e.ttl)
		if err != nil {
			e.logger.Warn("renewing leader lease", "key", e.key, "error", err)
			e.lock = nil
			return false
		}
		if renewed {
			return true
		}
		e.logger.Warn("lost leader lease to another replica", "key", e.key)
		e.lock = nil
	}

	lock, ok, err := e.client.AcquireLock(ctx, e.key, e.ttl)
	if err != nil {
		e.logger.Warn("acquiring leader lease", "key", e.key, "error", err)
		return false
	}
	if !ok {
		return false
	}
	e.logger.Info("acquired leader lease", "key", e.key)
	e.lock = lock
	return true
}

// Resign releases the lease immediately if held, so another replica can
// take over without waiting for the TTL to expire — call this on graceful
// shutdown.
func (e *Elector) Resign(ctx context.Context) {
	if e.lock == nil {
		return
	}
	if err := e.lock.Unlock(ctx); err != nil {
		e.logger.Warn("releasing leader lease", "key", e.key, "error", err)
	}
	e.lock = nil
}
