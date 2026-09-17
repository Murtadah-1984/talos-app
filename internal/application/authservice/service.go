// Package authservice implements the dev-mode login use case and the
// identity-to-platform-user resolution shared by every authenticated
// request (§26). In OIDC mode, callers never hit Login — the HTTP
// middleware resolves an Identity from the bearer token directly and this
// service only performs GetOrCreateUser.
package authservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
)

// TokenIssuer is implemented by auth.DevIdentityProvider; kept as a narrow
// interface here so this package doesn't depend on internal/auth directly.
type TokenIssuer interface {
	IssueToken(subject, email, name string, ttl time.Duration) (string, error)
}

type Service struct {
	users  user.Repository
	issuer TokenIssuer // nil in OIDC mode
}

func New(users user.Repository, issuer TokenIssuer) *Service {
	return &Service{users: users, issuer: issuer}
}

// GetOrCreateUser resolves an authenticated Identity into a platform User
// record, creating one on first login. This is the only place a User row is
// created from an external identity.
func (s *Service) GetOrCreateUser(ctx context.Context, identity ports.Identity) (*user.User, error) {
	u, err := s.users.GetBySubject(ctx, identity.Subject)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("looking up user by subject: %w", err)
	}

	u = &user.User{
		ID:      shared.NewID(),
		Email:   identity.Email,
		Name:    identity.Name,
		Subject: identity.Subject,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return u, nil
}

// DevLogin issues a locally-signed token for email/name (dev mode only,
// §26/ADR-none: gated entirely by PLATFORM_AUTH_MODE=dev at the transport
// layer). It ensures a corresponding User row exists so RBAC checks have
// something to bind role assignments to.
func (s *Service) DevLogin(ctx context.Context, email, name string) (string, *user.User, error) {
	if s.issuer == nil {
		return "", nil, fmt.Errorf("%w: dev login is not available in this auth mode", shared.ErrNotImplemented)
	}
	subject := "dev|" + email
	u, err := s.GetOrCreateUser(ctx, ports.Identity{Subject: subject, Email: email, Name: name})
	if err != nil {
		return "", nil, err
	}
	token, err := s.issuer.IssueToken(subject, email, name, 12*time.Hour)
	if err != nil {
		return "", nil, fmt.Errorf("issuing token: %w", err)
	}
	return token, u, nil
}
