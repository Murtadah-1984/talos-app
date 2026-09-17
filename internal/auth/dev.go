// Package auth implements authentication (identity verification) and RBAC
// authorization helpers. DevIdentityProvider issues and verifies locally
// signed tokens for local development; production deployments configure
// OIDC (Keycloak, Azure AD/Entra ID, GitHub) against the same
// ports.IdentityProvider interface (§26).
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// DevIdentityProvider issues and verifies HS256 JWTs signed with a shared
// secret. It exists solely so the platform is runnable and testable without
// standing up a real OIDC provider; it must never be enabled with
// PLATFORM_AUTH_MODE=dev in production (the config loader flags the mode
// explicitly so this is a deliberate, visible choice, not a silent default).
type DevIdentityProvider struct {
	signingKey []byte
}

func NewDevIdentityProvider(signingKey string) *DevIdentityProvider {
	return &DevIdentityProvider{signingKey: []byte(signingKey)}
}

type devClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	jwt.RegisteredClaims
}

// IssueToken creates a bearer token for subject/email/name, valid for ttl.
// Used by the dev-mode /auth/login endpoint only.
func (p *DevIdentityProvider) IssueToken(subject, email, name string, ttl time.Duration) (string, error) {
	claims := devClaims{
		Email: email,
		Name:  name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(p.signingKey)
}

func (p *DevIdentityProvider) Verify(_ context.Context, tokenString string) (ports.Identity, error) {
	claims := &devClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return p.signingKey, nil
	})
	if err != nil {
		return ports.Identity{}, fmt.Errorf("verifying dev token: %w", err)
	}
	if !token.Valid {
		return ports.Identity{}, errors.New("invalid token")
	}
	return ports.Identity{Subject: claims.Subject, Email: claims.Email, Name: claims.Name}, nil
}
