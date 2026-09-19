package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// OIDCIdentityProvider verifies bearer ID tokens issued by a real OIDC
// provider (Keycloak, Azure AD/Entra ID, GitHub, etc.), backing
// PLATFORM_AUTH_MODE=oidc — the production path (§26). The frontend
// completes the OIDC login flow itself (authorization code + PKCE) and
// sends the resulting ID token as a bearer credential; this type never
// handles user passwords or the authorization code exchange.
type OIDCIdentityProvider struct {
	verifier *oidc.IDTokenVerifier
}

// NewOIDCIdentityProvider performs OIDC discovery against issuer (fetching
// its JWKS for signature verification, with automatic key rotation handling
// via the underlying provider) and builds a verifier that requires tokens be
// issued for clientID.
func NewOIDCIdentityProvider(ctx context.Context, issuer, clientID string) (*OIDCIdentityProvider, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discovering OIDC provider at %s: %w", issuer, err)
	}
	return &OIDCIdentityProvider{
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

type oidcClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Verify checks the token's signature, issuer, audience, and expiry, then
// resolves it to a platform Identity. The subject is whatever the upstream
// OIDC provider considers stable and unique for the principal (its own
// "sub" claim) — the platform never mints its own subject identifiers for
// OIDC-authenticated users.
func (p *OIDCIdentityProvider) Verify(ctx context.Context, tokenString string) (ports.Identity, error) {
	idToken, err := p.verifier.Verify(ctx, tokenString)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("verifying OIDC token: %w", err)
	}
	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return ports.Identity{}, fmt.Errorf("decoding OIDC claims: %w", err)
	}
	return ports.Identity{Subject: idToken.Subject, Email: claims.Email, Name: claims.Name}, nil
}
