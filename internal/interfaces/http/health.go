package http

import "net/http"

// healthzHandler is a liveness probe: it never checks dependencies, only
// that the process can serve HTTP at all.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyzHandler is a readiness probe. Phase 1 reports process readiness;
// as dependency health checks (Postgres/Redis/RabbitMQ ping) land they
// should be added here rather than a separate endpoint, so orchestrators
// have one place to ask "can this instance take traffic".
func readyzHandler(_ Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
