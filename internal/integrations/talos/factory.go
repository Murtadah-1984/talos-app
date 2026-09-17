package talos

import (
	"fmt"
	"os"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// NewFromConfig builds the process-wide ports.TalosClient adapter: the
// deterministic mock when adapterMode is "mock" (the default, §44), or the
// real client (this file's Client) loaded from talosConfigFile when
// adapterMode is "real". Shared by cmd/platform-api and cmd/platform-worker
// so both processes select the adapter identically.
func NewFromConfig(adapterMode, talosConfigFile string) (ports.TalosClient, error) {
	switch adapterMode {
	case "mock", "":
		return NewMockClient(), nil
	case "real":
		data, err := os.ReadFile(talosConfigFile)
		if err != nil {
			return nil, fmt.Errorf("reading PLATFORM_TALOS_CONFIG_FILE %s: %w", talosConfigFile, err)
		}
		return NewClient(data)
	default:
		return nil, fmt.Errorf("unknown Talos adapter mode %q", adapterMode)
	}
}
