package ports

import "context"

// SecretRef identifies a secret in the configured SecretStore backend without
// revealing its value (ADR-0006).
type SecretRef struct {
	Backend string // "vault", "local-encrypted"
	Path    string
}

// SecretStore is the only path by which any component reads or writes
// sensitive material (Talos PKI, kubeconfigs, provider/GitHub credentials).
// Implementations: internal/infrastructure/vault (production), a local
// encrypted-at-rest implementation (development).
type SecretStore interface {
	Put(ctx context.Context, ref SecretRef, value []byte) error
	Get(ctx context.Context, ref SecretRef) ([]byte, error)
	Delete(ctx context.Context, ref SecretRef) error
}
