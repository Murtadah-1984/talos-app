package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
)

func mountWorkflows(r chi.Router, d Deps) {
	r.Route("/workflows", func(sub chi.Router) {
		sub.Get("/", func(w http.ResponseWriter, r *http.Request) {
			filter := workflowFilterFromQuery(r)
			workflows, err := d.Workflows.List(r.Context(), filter, pageFromQuery(r))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, workflows)
		})

		sub.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			wf, err := d.Workflows.Get(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			steps, err := d.Workflows.ListSteps(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"workflow": wf, "steps": steps})
		})
	})
}

func workflowFilterFromQuery(r *http.Request) workflow.Filter {
	q := r.URL.Query()
	var filter workflow.Filter
	if v := q.Get("clusterId"); v != "" {
		if id, err := shared.ParseID(v); err == nil {
			filter.ClusterID = &id
		}
	}
	filter.Type = workflow.Type(q.Get("type"))
	filter.Status = workflow.Status(q.Get("status"))
	return filter
}
