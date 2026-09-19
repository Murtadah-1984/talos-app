package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
	appmiddleware "github.com/talos-platform/talos-platform/internal/interfaces/http/middleware"
)

// recordAudit writes one audit.Log entry (§32: who/what/when/target/result)
// for a mutating action. Every destructive/mutating handler calls this
// after the action completes (or fails) — see requireRoleAudited for the
// authorization-denied case, which is audited at the choke point instead of
// in each handler.
//
// Logging is best-effort: a failure to write the audit record never fails
// the request it's describing, but is itself logged so a broken audit
// pipeline doesn't go unnoticed.
func recordAudit(ctx context.Context, d Deps, r *http.Request, actor *user.User, action, targetKind, targetID string, result audit.Result, errDetail string) {
	if d.Audit == nil {
		return
	}
	log := &audit.Log{
		RequestID:  appmiddleware.RequestIDFromContext(ctx),
		Action:     action,
		TargetKind: targetKind,
		TargetID:   targetID,
		Result:     result,
		IPAddress:  clientIP(r),
		OccurredAt: time.Now().UTC(),
	}
	if actor != nil {
		log.ActorID = actor.ID
		log.ActorEmail = actor.Email
	}
	if errDetail != "" {
		log.After = map[string]any{"error": errDetail}
	}
	if err := d.Audit.RecordLog(ctx, log); err != nil {
		d.Logger.Error("recording audit log", "action", action, "target_kind", targetKind, "target_id", targetID, "error", err)
	}
}

func auditResult(err error) audit.Result {
	if err != nil {
		return audit.ResultFailure
	}
	return audit.ResultSuccess
}

// errString safely renders err for an audit log's after-state, tolerating a
// nil error (the success case).
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// clientIP prefers X-Forwarded-For (set by a reverse proxy/load balancer in
// front of platform-api) over RemoteAddr, taking the first (client-nearest)
// entry when a chain of proxies is present.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	return r.RemoteAddr
}

// requireRoleAudited wraps requireRole (RBAC check) and additionally
// records a DENIED audit entry when authorization fails — access decisions
// are themselves security-relevant events worth a permanent record (§25,
// §32), not just an HTTP error response.
func requireRoleAudited(r *http.Request, d Deps, kind user.ResourceKind, id shared.ID, role user.Role, action, targetKind, targetID string) (*user.User, error) {
	u, err := requireRole(r, d, kind, id, role)
	if err != nil {
		actor, _ := appmiddleware.UserFromContext(r.Context())
		if errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrUnauthorized) {
			recordAudit(r.Context(), d, r, actor, action, targetKind, targetID, audit.ResultDenied, err.Error())
		}
		return nil, err
	}
	return u, nil
}
