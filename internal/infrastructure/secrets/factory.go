package secrets

import (
	"fmt"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// NewFromConfig builds the process-wide ports.SecretStore: the local
// AES-GCM-at-rest backend for "local" (development only — ADR-0006), or
// the real Vault-backed store for "vault".
func NewFromConfig(backend, vaultAddr, vaultToken, vaultMount, localKeyHex string) (ports.SecretStore, error) {
	switch backend {
	case "local", "":
		return NewLocalStore(localKeyHex)
	case "vault":
		if vaultAddr == "" || vaultToken == "" || vaultMount == "" {
			return nil, fmt.Errorf("PLATFORM_SECRETSTORE_BACKEND=vault requires PLATFORM_VAULT_ADDR, PLATFORM_VAULT_TOKEN, and PLATFORM_VAULT_MOUNT_PATH")
		}
		return NewVaultStore(vaultAddr, vaultToken, vaultMount)
	default:
		return nil, fmt.Errorf("unknown secret store backend %q", backend)
	}
}
