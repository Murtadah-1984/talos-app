package ports

import "context"

// Identity is the authenticated principal resolved from an inbound request's
// credential, independent of which IdentityProvider verified it.
type Identity struct {
	Subject string
	Email   string
	Name    string
}

// IdentityProvider verifies an inbound bearer credential and resolves the
// caller's identity. OIDC (Keycloak, Azure AD/Entra ID, GitHub via OIDC) is
// the target production implementation; a dev provider issuing/verifying
// locally-signed tokens backs local development (§26).
type IdentityProvider interface {
	Verify(ctx context.Context, token string) (Identity, error)
}
