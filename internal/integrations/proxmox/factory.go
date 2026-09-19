package proxmox

import (
	"fmt"
	"strconv"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// NewFromConfig builds the process-wide Proxmox ports.InfrastructureProvider
// adapter: the deterministic mock when adapterMode is "mock" (the default,
// §44), or the real client when adapterMode is "real".
func NewFromConfig(adapterMode, apiURL, node, apiToken, templateVMID string) (ports.InfrastructureProvider, error) {
	switch adapterMode {
	case "mock", "":
		return NewMockProvider(), nil
	case "real":
		if apiURL == "" || node == "" || apiToken == "" || templateVMID == "" {
			return nil, fmt.Errorf("PLATFORM_PROXMOX_ADAPTER=real requires PLATFORM_PROXMOX_API_URL, PLATFORM_PROXMOX_NODE, PLATFORM_PROXMOX_API_TOKEN, and PLATFORM_PROXMOX_TEMPLATE_VMID")
		}
		vmid, err := strconv.Atoi(templateVMID)
		if err != nil {
			return nil, fmt.Errorf("invalid PLATFORM_PROXMOX_TEMPLATE_VMID %q: %w", templateVMID, err)
		}
		return NewClient(apiURL, node, apiToken, vmid), nil
	default:
		return nil, fmt.Errorf("unknown Proxmox adapter mode %q", adapterMode)
	}
}
