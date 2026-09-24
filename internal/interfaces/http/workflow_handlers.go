package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
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

		sub.Post("/{id}/resume", func(w http.ResponseWriter, r *http.Request) {
			id, err := shared.ParseID(chi.URLParam(r, "id"))
			if err != nil {
				writeError(w, shared.ErrInvalidInput)
				return
			}
			kind, scopeID := workflowScope(r, d, id)
			u, err := requireRoleAudited(r, d, kind, scopeID, user.RoleOperator, "workflow.resume", "workflow", id.String())
			if err != nil {
				writeError(w, err)
				return
			}
			resumeErr := d.WorkflowEngine.Resume(r.Context(), id)
			recordAudit(r.Context(), d, r, u, "workflow.resume", "workflow", id.String(), auditResult(resumeErr), errString(resumeErr))
			if resumeErr != nil {
				writeError(w, resumeErr)
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "resumed"})
		})
	})
}

// workflowScope resolves the RBAC scope for a workflow-targeted action:
// CLUSTER scope when the workflow is associated with one (§4 approval gate
// resumption is an operator action on that cluster's change), falling back
// to PLATFORM scope for workflows with no associated cluster.
func workflowScope(r *http.Request, d Deps, workflowID shared.ID) (user.ResourceKind, shared.ID) {
	wf, err := d.Workflows.Get(r.Context(), workflowID)
	if err != nil || wf.ClusterID == nil {
		return user.ResourcePlatform, shared.ID{}
	}
	return user.ResourceCluster, *wf.ClusterID
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
