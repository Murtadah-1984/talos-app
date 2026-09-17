// Package middleware holds cross-cutting HTTP concerns: request ID,
// structured logging, authentication, and RBAC enforcement.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/talos-platform/talos-platform/internal/application/authservice"
	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/user"
)

type ctxKey string

const userCtxKey ctxKey = "platform.user"

// UserFromContext returns the authenticated user attached by Authenticate,
// panicking-free: callers should only call this on routes mounted behind
// Authenticate, where its presence is guaranteed.
func UserFromContext(ctx context.Context) (*user.User, bool) {
	u, ok := ctx.Value(userCtxKey).(*user.User)
	return u, ok
}

// Authenticate verifies the request's bearer token via identityProvider,
// resolves it to a platform user, and attaches that user to the request
// context. Requests without a valid token get 401 (§26, §48: the frontend
// never receives anything more sensitive than this token).
func Authenticate(identityProvider ports.IdentityProvider, auth *authservice.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}

			identity, err := identityProvider.Verify(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			u, err := auth.GetOrCreateUser(r.Context(), identity)
			if err != nil {
				http.Error(w, `{"error":"resolving user"}`, http.StatusInternalServerError)
				return
			}
			if u.Disabled {
				http.Error(w, `{"error":"account disabled"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), userCtxKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
