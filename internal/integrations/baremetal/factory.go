package baremetal

import (
	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/machine"
)

// NewProviderFromConfig builds a bare-metal Provider with real IPMI/Redfish
// power controllers registered for whichever protocols are enabled — a
// protocol left disabled falls back to NoopPowerController (NOT_CONFIGURED),
// per §44/ADR-0007, rather than silently no-op-ing. Both can be enabled at
// once: a single site's inventory may mix IPMI and Redfish machines,
// dispatched per-machine by Provider (see provider.go).
func NewProviderFromConfig(secrets ports.SecretStore, enableIPMI, enableRedfish bool) *Provider {
	controllers := map[machine.BMCProtocol]PowerController{}
	if enableIPMI {
		controllers[machine.BMCIPMI] = NewIPMIController(secrets)
	}
	if enableRedfish {
		controllers[machine.BMCRedfish] = NewRedfishController(secrets)
	}
	return NewProvider(controllers)
}
