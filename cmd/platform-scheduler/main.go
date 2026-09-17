// Command platform-scheduler runs periodic, non-workflow background jobs:
// today, a reconciliation pass that compares each cluster's desired state
// against observed Argo CD status and updates health metrics/alerts (§21,
// §31). It never reconciles anything itself (ADR-0003) — only observes and
// reports.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/infrastructure/config"
	"github.com/talos-platform/talos-platform/internal/infrastructure/postgres"
	"github.com/talos-platform/talos-platform/internal/observability"
	"github.com/talos-platform/talos-platform/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("loading configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := postgres.Migrate(cfg.Postgres.DSN, migrations.FS, "."); err != nil {
		logger.Error("applying database migrations", "error", err)
		os.Exit(1)
	}

	pool, err := postgres.NewPool(ctx, cfg.Postgres)
	if err != nil {
		logger.Error("connecting to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	clusters := postgres.NewClusterRepository(pool)
	auditRepo := postgres.NewAuditRepository(pool)
	metrics := observability.NewMetrics()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	logger.Info("platform-scheduler started")
	reconcile(ctx, logger, clusters, auditRepo, metrics)
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down platform-scheduler")
			return
		case <-ticker.C:
			reconcile(ctx, logger, clusters, auditRepo, metrics)
		}
	}
}

// reconcile keeps cluster_health gauges and drift-warning events fresh from
// the persisted Cluster.State (§21, §31). It is a visibility pass only —
// Argo CD/CAPI remain the systems that actually reconcile anything
// (ADR-0003); a future phase adds a live Argo CD Application health read
// here alongside the state-derived gauge below.
func reconcile(ctx context.Context, logger *slog.Logger, clusters cluster.Repository, auditRepo audit.Repository, metrics *observability.Metrics) {
	list, err := clusters.List(ctx, cluster.Filter{}, shared.Page{Limit: 500})
	if err != nil {
		logger.Error("listing clusters for reconciliation", "error", err)
		return
	}
	for _, c := range list {
		healthy := 0.0
		if c.State == cluster.StateReady {
			healthy = 1.0
		}
		metrics.ClusterHealth.WithLabelValues(c.ID.String(), c.Name).Set(healthy)
		if c.State == cluster.StateDegraded || c.State == cluster.StateFailed {
			_ = auditRepo.RecordEvent(ctx, &audit.Event{
				Source:     "platform-scheduler",
				Kind:       "reconciliation",
				Severity:   audit.SeverityWarning,
				TargetKind: "cluster",
				TargetID:   c.ID.String(),
				Message:    "cluster observed in " + string(c.State) + " state during reconciliation pass",
				OccurredAt: time.Now().UTC(),
			})
		}
	}
}
