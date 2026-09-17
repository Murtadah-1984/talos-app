package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

func mountAudit(r chi.Router, d Deps) {
	r.Get("/audit", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := audit.LogFilter{
			TargetKind: q.Get("targetKind"),
			TargetID:   q.Get("targetId"),
			Result:     audit.Result(q.Get("result")),
		}
		if v := q.Get("actorId"); v != "" {
			if id, err := shared.ParseID(v); err == nil {
				filter.ActorID = &id
			}
		}
		logs, err := d.Audit.ListLogs(r.Context(), filter, pageFromQuery(r))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, logs)
	})
}
