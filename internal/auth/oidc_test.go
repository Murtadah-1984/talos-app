package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// fakeOIDCServer serves a minimal OIDC discovery document and JWKS endpoint
// backed by a freshly generated RSA key, so OIDCIdentityProvider can be
// tested without a real identity provider.
type fakeOIDCServer struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	keyID  string
	issuer string
}

func newFakeOIDCServer(t *testing.T) *fakeOIDCServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	f := &fakeOIDCServer{key: key, keyID: "test-key-1"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   f.issuer,
			"jwks_uri": f.issuer + "/keys",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       &f.key.PublicKey,
			KeyID:     f.keyID,
			Algorithm: "RS256",
			Use:       "sig",
		}}}
		_ = json.NewEncoder(w).Encode(set)
	})

	f.srv = httptest.NewServer(mux)
	f.issuer = f.srv.URL
	return f
}

func (f *fakeOIDCServer) close() { f.srv.Close() }

func (f *fakeOIDCServer) issueToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       f.key,
	}, (&jose.SignerOptions{}).WithHeader("kid", f.keyID))
	if err != nil {
		t.Fatalf("creating signer: %v", err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("serializing token: %v", err)
	}
	return token
}

func TestOIDCIdentityProvider_VerifyValidToken(t *testing.T) {
	fake := newFakeOIDCServer(t)
	defer fake.close()

	provider, err := NewOIDCIdentityProvider(context.Background(), fake.issuer, "platform-client")
	if err != nil {
		t.Fatalf("constructing provider: %v", err)
	}

	now := time.Now()
	token := fake.issueToken(t, map[string]any{
		"iss":   fake.issuer,
		"aud":   "platform-client",
		"sub":   "user-123",
		"email": "alice@example.com",
		"name":  "Alice",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})

	identity, err := provider.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.Subject != "user-123" || identity.Email != "alice@example.com" || identity.Name != "Alice" {
		t.Errorf("unexpected identity: %+v", identity)
	}
}

func TestOIDCIdentityProvider_RejectsWrongAudience(t *testing.T) {
	fake := newFakeOIDCServer(t)
	defer fake.close()

	provider, err := NewOIDCIdentityProvider(context.Background(), fake.issuer, "platform-client")
	if err != nil {
		t.Fatalf("constructing provider: %v", err)
	}

	now := time.Now()
	token := fake.issueToken(t, map[string]any{
		"iss": fake.issuer,
		"aud": "some-other-client",
		"sub": "user-123",
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	})

	if _, err := provider.Verify(context.Background(), token); err == nil {
		t.Fatal("expected an error for a token issued to a different audience")
	}
}

func TestOIDCIdentityProvider_RejectsExpiredToken(t *testing.T) {
	fake := newFakeOIDCServer(t)
	defer fake.close()

	provider, err := NewOIDCIdentityProvider(context.Background(), fake.issuer, "platform-client")
	if err != nil {
		t.Fatalf("constructing provider: %v", err)
	}

	now := time.Now()
	token := fake.issueToken(t, map[string]any{
		"iss": fake.issuer,
		"aud": "platform-client",
		"sub": "user-123",
		"iat": now.Add(-2 * time.Hour).Unix(),
		"exp": now.Add(-time.Hour).Unix(),
	})

	if _, err := provider.Verify(context.Background(), token); err == nil {
		t.Fatal("expected an error for an expired token")
	}
}
