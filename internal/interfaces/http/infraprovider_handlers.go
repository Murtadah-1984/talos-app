package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/infraprovider"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

func mountInfraProviders(r chi.Router, d Deps) {
	r.Route("/infrastructure-providers", func(sub chi.Router) {
		sub.Get("/", func(w http.ResponseWriter, r *http.Request) {
			orgID, err := shared.ParseID(r.URL.Query().Get("organizationId"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			providers, err := d.InfraProviders.ListByOrganization(r.Context(), orgID, pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, providers)
		})

		sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var in infraprovider.InfrastructureProvider
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, err)
				return
			}
			if err := d.InfraProviders.Create(r.Context(), &in); err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, in)
		})

		sub.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			p, err := d.InfraProviders.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, p)
		})
	})
}
