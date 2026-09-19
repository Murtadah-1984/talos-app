package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimit applies a per-client-IP token bucket (§25 "rate limiting").
// Each distinct IP gets its own bucket refilling at rps requests/second
// with room to burst up to burst requests, so one noisy or malicious client
// can't starve everyone else, without needing a shared store (Redis) for
// what's inherently a best-effort, per-instance protection — a client
// spread across multiple platform-api replicas gets one bucket per replica,
// an acceptable trade-off for this layer of defense (it's not the source of
// truth for tenant-level quotas). The per-IP map grows for the process
// lifetime and is never pruned; acceptable at the scale this guards against
// (abusive/malfunctioning clients), not sized for tracking millions of
// unique callers.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	limiters := &limiterStore{
		byIP:  make(map[string]*rate.Limiter),
		rps:   rate.Limit(rps),
		burst: burst,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiters.forIP(clientIPForRateLimit(r)).Allow() {
				w.Header().Set("Retry-After", "1")
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type limiterStore struct {
	mu    sync.Mutex
	byIP  map[string]*rate.Limiter
	rps   rate.Limit
	burst int
}

func (s *limiterStore) forIP(ip string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.byIP[ip]
	if !ok {
		l = rate.NewLimiter(s.rps, s.burst)
		s.byIP[ip] = l
	}
	return l
}

// clientIPForRateLimit prefers X-Forwarded-For (set by a reverse proxy/load
// balancer in front of platform-api) over RemoteAddr's host, taking the
// first (client-nearest) entry when a chain of proxies is present.
func clientIPForRateLimit(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
