package argocd

import (
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// NewFromConfig builds the process-wide ports.ArgoCDClient adapter: the
// deterministic mock when adapterMode is "mock" (the default, §44), or the
// real REST client when adapterMode is "real".
func NewFromConfig(adapterMode, serverURL, token string) (ports.ArgoCDClient, error) {
	switch adapterMode {
	case "mock", "":
		return NewMockClient(), nil
	case "real":
		if serverURL == "" || token == "" {
			return nil, fmt.Errorf("PLATFORM_ARGOCD_ADAPTER=real requires PLATFORM_ARGOCD_SERVER_URL and PLATFORM_ARGOCD_TOKEN")
		}
		return NewClient(serverURL, token), nil
	default:
		return nil, fmt.Errorf("unknown Argo CD adapter mode %q", adapterMode)
	}
}
