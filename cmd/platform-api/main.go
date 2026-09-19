// Command platform-api serves the REST API described in §27: it wires
// configuration, Postgres, the workflow engine (dispatch-only — execution
// happens in platform-worker), and every HTTP route behind authentication.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/authservice"
	"github.com/talos-platform/talos-platform/internal/application/clusterservice"
	"github.com/talos-platform/talos-platform/internal/application/machineservice"
	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/auth"
	"github.com/talos-platform/talos-platform/internal/infrastructure/config"
	"github.com/talos-platform/talos-platform/internal/infrastructure/inprocess"
	"github.com/talos-platform/talos-platform/internal/infrastructure/postgres"
	"github.com/talos-platform/talos-platform/internal/infrastructure/rabbitmq"
	"github.com/talos-platform/talos-platform/internal/integrations/argocd"
	"github.com/talos-platform/talos-platform/internal/integrations/clusterapi"
	"github.com/talos-platform/talos-platform/internal/integrations/github"
	"github.com/talos-platform/talos-platform/internal/integrations/talos"
	httpapi "github.com/talos-platform/talos-platform/internal/interfaces/http"
	"github.com/talos-platform/talos-platform/internal/interfaces/websocket"
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

	shutdownTracing, err := observability.InitTracing(ctx, cfg.Observability.ServiceName, cfg.Observability.OTLPEndpoint, cfg.Observability.TracingEnabled)
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

	var identityProvider ports.IdentityProvider
	var issuer *auth.DevIdentityProvider
	if cfg.Auth.Mode == "dev" {
		issuer = auth.NewDevIdentityProvider(cfg.Auth.DevSigningKey)
		identityProvider = issuer
	} else {
		logger.Error("PLATFORM_AUTH_MODE=oidc is not yet implemented (see docs/roadmap.md)", "mode", cfg.Auth.Mode)
		os.Exit(1)
	}

	organizations := postgres.NewOrganizationRepository(pool)
	projects := postgres.NewProjectRepository(pool)
	environments := postgres.NewEnvironmentRepository(pool)
	sites := postgres.NewSiteRepository(pool)
	users := postgres.NewUserRepository(pool)
	templates := postgres.NewTemplateRepository(pool)
	infraProviders := postgres.NewInfraProviderRepository(pool)
	gitopsRepo := postgres.NewGitOpsRepository(pool)
	workflowRepo := postgres.NewWorkflowRepository(pool)
	auditRepo := postgres.NewAuditRepository(pool)
	clusters := postgres.NewClusterRepository(pool)
	machines := postgres.NewMachineRepository(pool)
	operations := postgres.NewOperationRepository(pool)

	authz := auth.NewAuthorizer(users, clusters, environments, projects)

	var broker ports.Broker
	if mqBroker, err := rabbitmq.Dial(cfg.RabbitMQ.URL); err != nil {
		logger.Warn("rabbitmq unavailable, falling back to in-process broker (not durable across restarts)", "error", err)
		broker = inprocess.NewBroker()
	} else {
		broker = mqBroker
	}

	talosClient, err := talos.NewFromConfig(cfg.TalosAdapterMode, cfg.TalosConfigFile)
	if err != nil {
		logger.Error("constructing Talos client", "error", err)
		os.Exit(1)
	}
	gitProvider, err := github.NewFromConfig(cfg.GitHubAdapterMode, cfg.GitHubToken)
	if err != nil {
		logger.Error("constructing GitHub client", "error", err)
		os.Exit(1)
	}
	argoClient, err := argocd.NewFromConfig(cfg.ArgoCDAdapterMode, cfg.ArgoCDServerURL, cfg.ArgoCDToken)
	if err != nil {
		logger.Error("constructing Argo CD client", "error", err)
		os.Exit(1)
	}
	capiProvider, err := clusterapi.NewFromConfig(cfg.ClusterAPIAdapterMode, cfg.ClusterAPIKubeconfigFile)
	if err != nil {
		logger.Error("constructing Cluster API client", "error", err)
		os.Exit(1)
	}

	workflowDeps := workflows.ClusterProvisionDeps{
		Clusters: clusters, Machines: machines, GitOps: gitopsRepo,
		Talos: talosClient, Git: gitProvider, ArgoCD: argoClient, ClusterAPI: capiProvider,
	}

	engine := workflows.NewEngine(workflowRepo, broker, auditRepo)
	engine.Register(workflows.NewClusterProvisionDefinition(workflowDeps))
	engine.Register(workflows.NewClusterUpgradeDefinition(workflowDeps))
	engine.Register(workflows.NewWorkerScaleDefinition(workflowDeps))
	if err := engine.StartConsuming(ctx); err != nil {
		logger.Error("starting workflow dispatch consumer", "error", err)
		os.Exit(1)
	}
	defer engine.Stop()

	authSvc := authservice.New(users, issuer)
	clusterSvc := clusterservice.New(clusters, gitopsRepo, engine, argoClient)
	machineSvc := machineservice.New(machines, operations, talosClient)

	metrics := observability.NewMetrics()

	deps := httpapi.Deps{
		Logger:           logger,
		IdentityProvider: identityProvider,
		Authz:            authz,
		Organizations:    organizations,
		Projects:         projects,
		Environments:     environments,
		Sites:            sites,
		Users:            users,
		Templates:        templates,
		InfraProviders:   infraProviders,
		GitOps:           gitopsRepo,
		Workflows:        workflowRepo,
		Audit:            auditRepo,
		AuthService:      authSvc,
		ClusterService:   clusterSvc,
		MachineService:   machineSvc,
		Events:           websocket.NewHub(),
		MetricsHandler:   metrics.Handler(),
	}

	router := httpapi.NewRouter(deps)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("platform-api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down platform-api")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
