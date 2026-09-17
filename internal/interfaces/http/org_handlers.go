package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/environment"
	"github.com/talos-platform/talos-platform/internal/domain/organization"
	"github.com/talos-platform/talos-platform/internal/domain/project"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/site"
)

func mountOrganizations(r chi.Router, d Deps) {
	r.Route("/organizations", func(sub chi.Router) {
		sub.Get("/", func(w http.ResponseWriter, r *http.Request) {
			orgs, err := d.Organizations.List(r.Context(), pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, orgs)
		})

		sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var in organization.Organization
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, err)
				return
			}
			if err := d.Organizations.Create(r.Context(), &in); err != nil {
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
			org, err := d.Organizations.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, org)
		})

		sub.Get("/{id}/projects", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			projects, err := d.Projects.ListByOrganization(r.Context(), id, pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, projects)
		})

		sub.Get("/{id}/sites", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			sites, err := d.Sites.ListByOrganization(r.Context(), id, pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, sites)
		})
	})
}

func mountProjects(r chi.Router, d Deps) {
	r.Route("/projects", func(sub chi.Router) {
		sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var in project.Project
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, err)
				return
			}
			if err := d.Projects.Create(r.Context(), &in); err != nil {
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
			p, err := d.Projects.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, p)
		})

		sub.Get("/{id}/environments", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			envs, err := d.Environments.ListByProject(r.Context(), id, pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, envs)
		})
	})
}

func mountEnvironments(r chi.Router, d Deps) {
	r.Route("/environments", func(sub chi.Router) {
		sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var in environment.Environment
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, err)
				return
			}
			if err := d.Environments.Create(r.Context(), &in); err != nil {
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
			e, err := d.Environments.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, e)
		})
	})
}

func mountSites(r chi.Router, d Deps) {
	r.Route("/sites", func(sub chi.Router) {
		sub.Post("/", func(w http.ResponseWriter, r *http.Request) {
			var in site.Site
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, err)
				return
			}
			if err := d.Sites.Create(r.Context(), &in); err != nil {
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
			s, err := d.Sites.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, s)
		})
	})
}
