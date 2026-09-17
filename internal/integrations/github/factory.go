package github

import (
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// NewFromConfig builds the process-wide ports.GitProvider adapter: the
// deterministic mock when adapterMode is "mock" (the default, §44), or the
// real GitHub client when adapterMode is "real", authenticated with token
// (a personal access token or GitHub App installation token).
func NewFromConfig(adapterMode, token string) (ports.GitProvider, error) {
	switch adapterMode {
	case "mock", "":
		return NewMockProvider(), nil
	case "real":
		if token == "" {
			return nil, fmt.Errorf("PLATFORM_GITHUB_ADAPTER=real requires PLATFORM_GITHUB_TOKEN")
		}
		return NewClient(token), nil
	default:
		return nil, fmt.Errorf("unknown GitHub adapter mode %q", adapterMode)
	}
}
