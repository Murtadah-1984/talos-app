package clusterapi

import (
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// NewFromConfig builds the process-wide ports.ClusterAPIProvider adapter:
// the deterministic mock when adapterMode is "mock" (the default, §44), or
// the real client (backed by a kubeconfig for the CAPI management cluster)
// when adapterMode is "real". proxmox is the same global Proxmox
// configuration internal/integrations/proxmox uses for the real
// InfrastructureProvider client — its zero value disables Proxmox
// infrastructure CR rendering (RenderManifests falls back to core CAPI
// resources only).
func NewFromConfig(adapterMode, kubeconfigPath string, proxmox ProxmoxInfrastructure) (ports.ClusterAPIProvider, error) {
	switch adapterMode {
	case "mock", "":
		return NewMockProvider(), nil
	case "real":
		if kubeconfigPath == "" {
			return nil, fmt.Errorf("PLATFORM_CLUSTERAPI_ADAPTER=real requires PLATFORM_CLUSTERAPI_KUBECONFIG_FILE")
		}
		return NewClient(kubeconfigPath, proxmox)
	default:
		return nil, fmt.Errorf("unknown Cluster API adapter mode %q", adapterMode)
	}
}
