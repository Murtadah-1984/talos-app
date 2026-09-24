package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestValidGitHubWebhookSignature(t *testing.T) {
	secret := "test-webhook-secret"
	body := []byte(`{"action":"closed","pull_request":{"html_url":"https://github.com/acme/infra/pull/7","merged":true}}`)

	if !validGitHubWebhookSignature(secret, sign(secret, body), body) {
		t.Error("expected a correctly signed payload to validate")
	}
	if validGitHubWebhookSignature(secret, sign("wrong-secret", body), body) {
		t.Error("expected a payload signed with the wrong secret to be rejected")
	}
	if validGitHubWebhookSignature(secret, sign(secret, body), []byte("tampered body")) {
		t.Error("expected a mismatched body to be rejected")
	}
	if validGitHubWebhookSignature(secret, "", body) {
		t.Error("expected a missing signature header to be rejected")
	}
	if validGitHubWebhookSignature(secret, "sha1="+hex.EncodeToString([]byte("wrong-algo")), body) {
		t.Error("expected a non-sha256 signature header to be rejected")
	}
	if validGitHubWebhookSignature(secret, "sha256=not-hex", body) {
		t.Error("expected a non-hex signature to be rejected")
	}
}
