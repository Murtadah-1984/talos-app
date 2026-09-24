package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
)

// mountGitOps exposes GitOps repository/configuration metadata directly
// (distinct from the per-cluster status endpoint mounted under
// /clusters/{id}/gitops in cluster_handlers.go).
func mountGitOps(r chi.Router, d Deps) {
	r.Route("/gitops", func(sub chi.Router) {
		sub.Get("/repositories", func(w http.ResponseWriter, r *http.Request) {
			orgID, err := shared.ParseID(r.URL.Query().Get("organizationId"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			repos, err := d.GitOps.ListRepositories(r.Context(), orgID)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, repos)
		})

		sub.Post("/repositories/{id}/scaffold", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			u, err := requireRoleAudited(r, d, user.ResourcePlatform, shared.ID{}, user.RoleOperator, "gitops.scaffold_repository", "git_repository", id.String())
			if err != nil {
				writeError(w, err)
				return
			}
			scaffoldErr := d.ClusterService.ScaffoldRepository(r.Context(), id)
			recordAudit(r.Context(), d, r, u, "gitops.scaffold_repository", "git_repository", id.String(), auditResult(scaffoldErr), errString(scaffoldErr))
			if scaffoldErr != nil {
				writeError(w, scaffoldErr)
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "scaffolded"})
		})
	})
}
