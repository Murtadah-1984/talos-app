package baremetal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// bmcCredentials is the JSON shape expected in the SecretStore for a
// machine's BMC credentialRef — the same shape regardless of protocol
// (IPMI/Redfish both authenticate with a username/password pair).
type bmcCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// resolveCredentials loads and decodes a machine's BMC credentials. The
// SecretStore backend label is fixed per this package rather than
// per-machine: which concrete backend (local/Vault) is in play is a
// deployment-wide choice (ADR-0006), not something bare-metal inventory
// data should encode.
func resolveCredentials(ctx context.Context, secrets ports.SecretStore, credentialRef string) (bmcCredentials, error) {
	data, err := secrets.Get(ctx, ports.SecretRef{Backend: "baremetal", Path: credentialRef})
	if err != nil {
		return bmcCredentials{}, fmt.Errorf("resolving BMC credentials %q: %w", credentialRef, err)
	}
	var creds bmcCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return bmcCredentials{}, fmt.Errorf("decoding BMC credentials %q: %w", credentialRef, err)
	}
	return creds, nil
}
