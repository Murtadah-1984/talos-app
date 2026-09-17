package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/template"
)

func mountTemplates(r chi.Router, d Deps) {
	r.Route("/templates", func(sub chi.Router) {
		sub.Get("/", func(w http.ResponseWriter, r *http.Request) {
			orgID, err := shared.ParseID(r.URL.Query().Get("organizationId"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			templates, err := d.Templates.List(r.Context(), orgID, pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, templates)
		})

		// Templates are immutable once created (§13): a new version is a
		// new row, never an in-place update.
		sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var in template.ClusterTemplate
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, err)
				return
			}
			if err := d.Templates.Create(r.Context(), &in); err != nil {
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
			t, err := d.Templates.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, t)
		})

		sub.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			if err := d.Templates.Delete(r.Context(), id); err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusNoContent, nil)
		})
	})
}
