package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/application/clusterservice"
	"github.com/talos-platform/talos-platform/internal/domain/cluster"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
	appmiddleware "github.com/talos-platform/talos-platform/internal/interfaces/http/middleware"
)

func mountClusters(r chi.Router, d Deps) {
	r.Route("/clusters", func(sub chi.Router) {
		sub.Get("/", func(w http.ResponseWriter, r *http.Request) {
			filter := clusterFilterFromQuery(r)
			clusters, err := d.ClusterService.List(r.Context(), filter, pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, clusters)
		})

		sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var in cluster.Cluster
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, err)
				return
			}
			created, err := d.ClusterService.Create(r.Context(), clusterserviceCreateInput(&in))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, created)
		})

		sub.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			c, err := d.ClusterService.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, c)
		})

		sub.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			u, err := requireRole(r, d, user.ResourceCluster, id, user.RoleClusterAdmin)
			if err != nil {
				writeError(w, err)
				return
			}
			wf, err := d.ClusterService.Delete(r.Context(), id, u.ID, idempotencyKey(r, "delete-"+id.String()))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, wf)
		})

		sub.Post("/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			plan, err := d.ClusterService.Plan(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, plan)
		})

		sub.Post("/{id}/provision", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			u, err := requireRole(r, d, user.ResourceCluster, id, user.RoleOperator)
			if err != nil {
				writeError(w, err)
				return
			}
			wf, err := d.ClusterService.Provision(r.Context(), id, u.ID, idempotencyKey(r, "provision-"+id.String()))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, wf)
		})

		sub.Post("/{id}/upgrade", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			u, err := requireRole(r, d, user.ResourceCluster, id, user.RoleClusterAdmin)
			if err != nil {
				writeError(w, err)
				return
			}
			var req struct {
				KubernetesVersion string `json:"kubernetesVersion"`
				TalosVersion      string `json:"talosVersion"`
			}
			if err := decodeJSON(r, &req); err != nil {
				writeError(w, err)
				return
			}
			wf, err := d.ClusterService.Upgrade(r.Context(), id, u.ID, idempotencyKey(r, "upgrade-"+id.String()), req.KubernetesVersion, req.TalosVersion)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, wf)
		})

		sub.Post("/{id}/scale", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			u, err := requireRole(r, d, user.ResourceCluster, id, user.RoleOperator)
			if err != nil {
				writeError(w, err)
				return
			}
			var req struct {
				Pool     string `json:"pool"`
				Replicas int32  `json:"replicas"`
			}
			if err := decodeJSON(r, &req); err != nil {
				writeError(w, err)
				return
			}
			wf, err := d.ClusterService.ScaleWorkers(r.Context(), id, u.ID, idempotencyKey(r, "scale-"+id.String()), req.Pool, req.Replicas)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, wf)
		})

		sub.Get("/{id}/health", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			c, err := d.ClusterService.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"state": c.State})
		})

		sub.Get("/{id}/nodes", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			machines, err := d.MachineService.List(r.Context(), machineFilterForCluster(id), pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, machines)
		})

		sub.Get("/{id}/gitops", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			status, err := d.ClusterService.GitOpsStatus(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, status)
		})

		sub.Post("/{id}/sync", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			if _, err := requireRole(r, d, user.ResourceCluster, id, user.RoleOperator); err != nil {
				writeError(w, err)
				return
			}
			if err := d.ClusterService.TriggerSync(r.Context(), id); err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync triggered"})
		})
	})
}

// requireRole resolves the authenticated user and checks RBAC via
// d.Authz.Authorize (§26). It is the single choke point every mutating
// cluster route uses, so authorization can't accidentally be skipped.
func requireRole(r *http.Request, d Deps, kind user.ResourceKind, id shared.ID, role user.Role) (*user.User, error) {
	u, ok := appmiddleware.UserFromContext(r.Context())
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	if err := d.Authz.Authorize(r.Context(), u.ID, kind, id, role); err != nil {
		return nil, err
	}
	return u, nil
}

// idempotencyKey resolves the client-supplied Idempotency-Key header,
// falling back to a per-action default derived from the request so repeated
// calls without the header still collapse onto the same workflow (§34).
func idempotencyKey(r *http.Request, fallback string) string {
	if k := r.Header.Get("Idempotency-Key"); k != "" {
		return k
	}
	return fallback
}

func clusterFilterFromQuery(r *http.Request) cluster.Filter {
	q := r.URL.Query()
	var filter cluster.Filter
	if v := q.Get("organizationId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.OrganizationID = &id
		}
	}
	if v := q.Get("projectId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.ProjectID = &id
		}
	}
	if v := q.Get("environmentId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.EnvironmentID = &id
		}
	}
	if v := q.Get("siteId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.SiteID = &id
		}
	}
	filter.ProviderMode = cluster.ProviderMode(q.Get("providerMode"))
	filter.State = cluster.State(q.Get("state"))
	filter.KubernetesVersion = q.Get("kubernetesVersion")
	filter.TalosVersion = q.Get("talosVersion")
	return filter
}

func clusterserviceCreateInput(c *cluster.Cluster) clusterservice.CreateInput {
	return clusterservice.CreateInput{
		OrganizationID: c.OrganizationID,
		ProjectID:      c.ProjectID,
		EnvironmentID:  c.EnvironmentID,
		SiteID:         c.SiteID,
		TemplateID:     c.TemplateID,
		Name:           c.Name,
		ProviderMode:   c.ProviderMode,
		Spec:           c.Spec,
	}
}
