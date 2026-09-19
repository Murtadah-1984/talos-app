package middleware

import "net/http"

// SecurityHeaders sets a baseline set of defensive HTTP response headers
// (§25 security hardening) appropriate for a JSON API that is never
// expected to render attacker-controlled HTML: it never needs to be framed,
// never needs the browser to sniff content types, and never sends a
// Referer header carrying potentially sensitive query parameters onward.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		next.ServeHTTP(w, r)
	})
}

// CORS allows the configured web origin(s) to call the API with credentials
// (the browser sends the bearer token via an Authorization header, not a
// cookie, but the preflight/actual-request dance still applies for
// cross-origin fetches from the SPA — §48: the frontend only ever talks to
// the platform API). An empty allowedOrigins means same-origin only (no
// CORS headers emitted), the safest default when the frontend is served
// from the same origin as the API (e.g. behind one ingress).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; ok {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
				h.Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
