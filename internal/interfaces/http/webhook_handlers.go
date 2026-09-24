package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// maxWebhookBodyBytes bounds how much of a webhook delivery this handler
// will read, so an oversized or malicious payload can't exhaust memory.
const maxWebhookBodyBytes = 1 << 20 // 1 MiB, generous for a pull_request event

// mountWebhooks registers inbound webhook endpoints. Unlike every other
// route, these are mounted outside the authenticated group (server.go):
// GitHub calls them directly with no bearer token, authenticating instead
// via an HMAC signature over the raw body (verified below).
func mountWebhooks(r chi.Router, d Deps) {
	r.Post("/webhooks/github", githubWebhookHandler(d))
}

// githubPullRequestEvent is the subset of GitHub's pull_request webhook
// payload this handler needs. See
// https://docs.github.com/en/webhooks/webhook-events-and-payloads#pull_request.
type githubPullRequestEvent struct {
	Action      string `json:"action"`
	PullRequest struct {
		HTMLURL string `json:"html_url"`
		Merged  bool   `json:"merged"`
	} `json:"pull_request"`
}

// githubWebhookHandler implements the §4 human-in-the-loop approval gate's
// production path: a workflow paused in AWAITING_APPROVAL (see
// workflows.ErrAwaitingApproval) sits idle until a "pull request merged"
// event here resumes it — never a poll loop.
func githubWebhookHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.GitHubWebhookSecret == "" {
			http.Error(w, `{"error":"github webhooks are not configured"}`, http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes+1))
		if err != nil {
			writeError(w, shared.ErrInvalidInput)
			return
		}
		if len(body) > maxWebhookBodyBytes {
			http.Error(w, `{"error":"payload too large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		if !validGitHubWebhookSignature(d.GitHubWebhookSecret, r.Header.Get("X-Hub-Signature-256"), body) {
			http.Error(w, `{"error":"invalid webhook signature"}`, http.StatusUnauthorized)
			return
		}

		// GitHub sends many event types (push, issues, ping, ...) to the
		// same URL when a webhook is configured broadly; only pull_request
		// carries the merge signal this gate waits for.
		if r.Header.Get("X-GitHub-Event") != "pull_request" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		var event githubPullRequestEvent
		if err := json.Unmarshal(body, &event); err != nil {
			writeError(w, shared.ErrInvalidInput)
			return
		}
		if event.Action != "closed" || !event.PullRequest.Merged {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}

		changeset, err := d.GitOps.GetChangeSetByPullRequestURL(r.Context(), event.PullRequest.HTMLURL)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]string{"status": "no matching change set"})
				return
			}
			writeError(w, err)
			return
		}
		if changeset.WorkflowID == nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "change set has no associated workflow"})
			return
		}

		if err := d.WorkflowEngine.Resume(r.Context(), *changeset.WorkflowID); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
	}
}

// validGitHubWebhookSignature checks the X-Hub-Signature-256 header GitHub
// sends on every webhook delivery: "sha256=" followed by a hex-encoded
// HMAC-SHA256 of the raw request body, keyed with the webhook's configured
// secret. Comparison is constant-time to avoid a timing side channel.
func validGitHubWebhookSignature(secret, header string, body []byte) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
