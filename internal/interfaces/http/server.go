package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/talos-platform/talos-platform/internal/application/authservice"
	"github.com/talos-platform/talos-platform/internal/application/clusterservice"
	"github.com/talos-platform/talos-platform/internal/application/machineservice"
	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/auth"
	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/environment"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/infraprovider"
	"github.com/talos-platform/talos-platform/internal/domain/organization"
	"github.com/talos-platform/talos-platform/internal/domain/project"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/site"
	"github.com/talos-platform/talos-platform/internal/domain/template"
	"github.com/talos-platform/talos-platform/internal/domain/user"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
	appmiddleware "github.com/talos-platform/talos-platform/internal/interfaces/http/middleware"
	"github.com/talos-platform/talos-platform/internal/interfaces/websocket"
)

// WorkflowEngine is the narrow slice of *workflows.Engine this package
// needs — just enough for the GitHub webhook handler to resume a paused
// workflow (§4). Kept as a local interface (rather than importing
// internal/workflows directly) for the same reason
// clusterservice.WorkflowEnqueuer is: internal/workflows depends on
// packages this one also depends on, and Go forbids the resulting cycle.
type WorkflowEngine interface {
	Resume(ctx context.Context, id shared.ID) error
}

// Deps bundles every dependency the HTTP layer needs. Handlers are thin:
// they translate transport <-> application/domain calls and contain no
// business logic of their own.
type Deps struct {
	Logger *slog.Logger

	IdentityProvider ports.IdentityProvider
	Authz            *auth.Authorizer

	Organizations  organization.Repository
	Projects       project.Repository
	Environments   environment.Repository
	Sites          site.Repository
	Users          user.Repository
	Templates      template.Repository
	InfraProviders infraprovider.Repository
	GitOps         gitops.Repositories
	Workflows      workflow.Repository
	Audit          audit.Repository

	AuthService    *authservice.Service
	ClusterService *clusterservice.Service
	MachineService *machineservice.Service

	// WorkflowEngine resumes a workflow paused in AWAITING_APPROVAL; used
	// only by the GitHub webhook handler (§4).
	WorkflowEngine WorkflowEngine
	// GitHubWebhookSecret verifies inbound GitHub webhook deliveries; empty
	// disables the webhook endpoint (see webhook_handlers.go).
	GitHubWebhookSecret string

	Events *websocket.Hub

	MetricsHandler http.Handler

	// CORSOrigins are the web origins allowed to call this API cross-origin
	// (§48); empty means same-origin only. RateLimitRPS/RateLimitBurst
	// configure the per-client-IP rate limiter (§25); RateLimitRPS <= 0
	// disables rate limiting entirely (e.g. for local development).
	CORSOrigins    []string
	RateLimitRPS   float64
	RateLimitBurst int
}

// NewRouter builds the full chi router: middleware chain, health/readiness
// endpoints (unauthenticated), Prometheus metrics, and every resource route
// from §27, mounted behind authentication.
func NewRouter(d Deps) *chi.Mux { //nolint:revive // deps struct is intentional
	r := chi.NewRouter()

	r.Use(chimw.Recoverer)
	r.Use(appmiddleware.RequestID)
	r.Use(appmiddleware.Logging(d.Logger))
	r.Use(appmiddleware.SecurityHeaders)
	r.Use(appmiddleware.CORS(d.CORSOrigins))
	if d.RateLimitRPS > 0 {
		r.Use(appmiddleware.RateLimit(d.RateLimitRPS, d.RateLimitBurst))
	}

	r.Get("/healthz", healthzHandler)
	r.Get("/readyz", readyzHandler(d))
	if d.MetricsHandler != nil {
		r.Handle("/metrics", d.MetricsHandler)
	}

	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/login", devLoginHandler(d))
		mountWebhooks(api, d)

		api.Group(func(authed chi.Router) {
			authed.Use(appmiddleware.Authenticate(d.IdentityProvider, d.AuthService))

			mountOrganizations(authed, d)
			mountProjects(authed, d)
			mountEnvironments(authed, d)
			mountSites(authed, d)
			mountTemplates(authed, d)
			mountInfraProviders(authed, d)
			mountClusters(authed, d)
			mountMachines(authed, d)
			mountWorkflows(authed, d)
			mountAudit(authed, d)
			mountGitOps(authed, d)

			authed.Get("/events/stream", d.Events.ServeSSE)
		})
	})

	return r
}
