// Command platform-worker executes workflow steps dispatched by
// platform-api (ADR-0005). Multiple replicas can run safely: step claims are
// serialized in Postgres via SELECT ... FOR UPDATE SKIP LOCKED, not by
// anything in this process.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/infrastructure/config"
	"github.com/talos-platform/talos-platform/internal/infrastructure/inprocess"
	"github.com/talos-platform/talos-platform/internal/infrastructure/postgres"
	"github.com/talos-platform/talos-platform/internal/infrastructure/rabbitmq"
	"github.com/talos-platform/talos-platform/internal/integrations/argocd"
	"github.com/talos-platform/talos-platform/internal/integrations/clusterapi"
	"github.com/talos-platform/talos-platform/internal/integrations/github"
	"github.com/talos-platform/talos-platform/internal/observability"
	"github.com/talos-platform/talos-platform/internal/workflows"
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

	shutdownTracing, err := observability.InitTracing(ctx, "platform-worker", cfg.Observability.OTLPEndpoint, cfg.Observability.TracingEnabled)
	if err != nil {
		logger.Error("initializing tracing", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

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
	workflowRepo := postgres.NewWorkflowRepository(pool)
	auditRepo := postgres.NewAuditRepository(pool)

	var broker ports.Broker
	if mqBroker, err := rabbitmq.Dial(cfg.RabbitMQ.URL); err != nil {
		logger.Warn("rabbitmq unavailable, falling back to in-process broker (not durable across restarts)", "error", err)
		broker = inprocess.NewBroker()
	} else {
		broker = mqBroker
	}

	gitProvider := github.NewMockProvider()
	argoClient := argocd.NewMockClient()
	capiProvider := clusterapi.NewMockProvider()

	deps := workflows.ClusterProvisionDeps{
		Clusters: clusters, GitOps: gitopsRepo, Git: gitProvider, ArgoCD: argoClient, ClusterAPI: capiProvider,
	}

	engine := workflows.NewEngine(workflowRepo, broker, auditRepo)
	engine.Register(workflows.NewClusterProvisionDefinition(deps))
	engine.Register(workflows.NewClusterUpgradeDefinition(deps))
	engine.Register(workflows.NewWorkerScaleDefinition(deps))

	if err := engine.StartConsuming(ctx); err != nil {
		logger.Error("starting workflow dispatch consumer", "error", err)
		os.Exit(1)
	}
	defer engine.Stop()

	logger.Info("platform-worker started")
	<-ctx.Done()
	logger.Info("shutting down platform-worker")
}
