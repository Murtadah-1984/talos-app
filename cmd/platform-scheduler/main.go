// Command platform-scheduler runs periodic, non-workflow background jobs:
// a reconciliation pass that compares each cluster's desired state against
// observed Argo CD status and updates health metrics/alerts (§21, §31). It
// never reconciles anything itself (ADR-0003) — only observes and reports.
//
// Only one replica does this work at a time (§22, Phase 7 HA): a Redis
// lease elects a leader, so running multiple replicas for availability
// doesn't duplicate reconciliation ticks or emit duplicate alerts.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/infrastructure/config"
	"github.com/talos-platform/talos-platform/internal/infrastructure/leaderelection"
	"github.com/talos-platform/talos-platform/internal/infrastructure/postgres"
	"github.com/talos-platform/talos-platform/internal/infrastructure/redis"
	"github.com/talos-platform/talos-platform/internal/integrations/argocd"
	"github.com/talos-platform/talos-platform/internal/observability"
	"github.com/talos-platform/talos-platform/migrations"
)

const leaderLeaseTTL = 45 * time.Second

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
	gitopsRepo := postgres.NewGitOpsRepository(pool)
	auditRepo := postgres.NewAuditRepository(pool)
	metrics := observability.NewMetrics()

	argoClient, err := argocd.NewFromConfig(cfg.ArgoCDAdapterMode, cfg.ArgoCDServerURL, cfg.ArgoCDToken)
	if err != nil {
		logger.Error("constructing Argo CD client", "error", err)
		os.Exit(1)
	}

	r := &reconciler{clusters: clusters, gitops: gitopsRepo, audit: auditRepo, argo: argoClient, metrics: metrics, logger: logger}

	redisClient := redis.New(cfg.Redis.Addr)
	defer func() { _ = redisClient.Close() }()
	elector := leaderelection.New(redisClient, "platform-scheduler-leader", leaderLeaseTTL, logger)
	defer elector.Resign(context.Background())

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	logger.Info("platform-scheduler started")
	runIfLeader(ctx, elector, r)
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down platform-scheduler")
			return
		case <-ticker.C:
			runIfLeader(ctx, elector, r)
		}
	}
}

// runIfLeader gates the reconciliation pass on holding the leader lease, so
// running platform-scheduler with more than one replica for availability
// doesn't duplicate reconciliation work or emit duplicate alerts.
func runIfLeader(ctx context.Context, elector *leaderelection.Elector, r *reconciler) {
	if !elector.IsLeader(ctx) {
		return
	}
	r.run(ctx)
}

type reconciler struct {
	clusters cluster.Repository
	gitops   gitops.Repositories
	audit    audit.Repository
	argo     ports.ArgoCDClient
	metrics  *observability.Metrics
	logger   *slog.Logger
}

// run keeps cluster_health gauges, the cached Argo CD Application status
// (used by the cluster GitOps tab), and drift/health alerts fresh (§21,
// §31). It is a visibility pass only — Argo CD remains the system that
// actually reconciles anything (ADR-0003).
func (r *reconciler) run(ctx context.Context) {
	list, err := r.clusters.List(ctx, cluster.Filter{}, shared.Page{Limit: 500})
	if err != nil {
		r.logger.Error("listing clusters for reconciliation", "error", err)
		return
	}
	for _, c := range list {
		r.reconcileCluster(ctx, c)
	}
}

func (r *reconciler) reconcileCluster(ctx context.Context, c *cluster.Cluster) {
	healthy := 0.0
	if c.State == cluster.StateReady {
		healthy = 1.0
	}
	r.metrics.ClusterHealth.WithLabelValues(c.ID.String(), c.Name).Set(healthy)

	firing, err := r.firingAlertTitles(ctx, c.ID)
	if err != nil {
		r.logger.Error("listing firing alerts", "cluster", c.Name, "error", err)
		firing = map[string]bool{}
	}

	const clusterStateTitle = "Cluster in DEGRADED/FAILED state"
	if c.State == cluster.StateDegraded || c.State == cluster.StateFailed {
		r.upsertAlert(ctx, c.ID, clusterStateTitle, audit.SeverityWarning, audit.AlertFiring,
			"cluster observed in "+string(c.State)+" state during reconciliation pass")
	} else if firing[clusterStateTitle] {
		r.upsertAlert(ctx, c.ID, clusterStateTitle, audit.SeverityWarning, audit.AlertResolved, "")
	}

	if !c.Spec.ArgoCD.Enabled {
		return
	}
	r.reconcileArgoCD(ctx, c, firing)
}

// firingAlertTitles returns the titles of every currently-FIRING alert for
// clusterID, so reconcileCluster/reconcileArgoCD only write a RESOLVED
// transition for conditions that were actually firing — avoiding a
// RESOLVED row for every never-fired condition on every healthy cluster,
// on every tick.
func (r *reconciler) firingAlertTitles(ctx context.Context, clusterID shared.ID) (map[string]bool, error) {
	alerts, err := r.audit.ListAlerts(ctx, audit.AlertFilter{
		TargetKind: "cluster", TargetID: clusterID.String(), Status: audit.AlertFiring,
	}, shared.Page{Limit: 50})
	if err != nil {
		return nil, err
	}
	titles := make(map[string]bool, len(alerts))
	for _, a := range alerts {
		titles[a.Title] = true
	}
	return titles, nil
}

// reconcileArgoCD refreshes the gitops.ArgoApplication cache the cluster
// GitOps tab reads and raises/resolves drift and health alerts. An
// Application that doesn't exist yet in Argo CD (NotFound) is not an error —
// the commit that creates it may not have synced yet.
func (r *reconciler) reconcileArgoCD(ctx context.Context, c *cluster.Cluster, firing map[string]bool) {
	status, err := r.argo.GetApplication(ctx, c.Name)
	if err != nil {
		if !errors.Is(err, shared.ErrNotFound) {
			r.logger.Warn("checking argo cd application", "cluster", c.Name, "error", err)
		}
		return
	}

	if err := r.gitops.UpsertArgoApplication(ctx, &gitops.ArgoApplication{
		ClusterID:    c.ID,
		Name:         status.Name,
		Namespace:    status.Namespace,
		Project:      status.Project,
		SyncStatus:   status.SyncStatus,
		HealthStatus: status.HealthStatus,
		Revision:     status.Revision,
	}); err != nil {
		r.logger.Error("caching argo cd application status", "cluster", c.Name, "error", err)
	}

	const driftTitle = "GitOps drift detected"
	if status.SyncStatus == gitops.SyncStatusOutOfSync {
		r.upsertAlert(ctx, c.ID, driftTitle, audit.SeverityWarning, audit.AlertFiring,
			"Argo CD reports "+c.Name+" as OutOfSync (revision "+status.Revision+")")
	} else if firing[driftTitle] {
		r.upsertAlert(ctx, c.ID, driftTitle, audit.SeverityWarning, audit.AlertResolved, "")
	}

	const healthTitle = "Argo CD application unhealthy"
	if status.HealthStatus == gitops.HealthDegraded || status.HealthStatus == gitops.HealthMissing {
		r.upsertAlert(ctx, c.ID, healthTitle, audit.SeverityError, audit.AlertFiring,
			"Argo CD reports "+c.Name+" health as "+string(status.HealthStatus))
	} else if firing[healthTitle] {
		r.upsertAlert(ctx, c.ID, healthTitle, audit.SeverityError, audit.AlertResolved, "")
	}
}

func (r *reconciler) upsertAlert(ctx context.Context, clusterID shared.ID, title string, severity audit.EventSeverity, status audit.AlertStatus, detail string) {
	now := time.Now().UTC()
	alert := &audit.Alert{
		TargetKind: "cluster",
		TargetID:   clusterID.String(),
		Severity:   severity,
		Status:     status,
		Title:      title,
		Detail:     detail,
		FiredAt:    now,
	}
	if status == audit.AlertResolved {
		alert.ResolvedAt = &now
	}
	if err := r.audit.UpsertAlert(ctx, alert); err != nil {
		r.logger.Error("upserting alert", "title", title, "error", err)
	}
}
