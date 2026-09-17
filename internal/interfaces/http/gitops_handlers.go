package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
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
	})
}
