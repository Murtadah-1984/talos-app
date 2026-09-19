// Package secrets implements the ports.SecretStore port (ADR-0006). This
// file is the production backend: HashiCorp Vault's KV v2 secrets engine,
// via the official Vault Go client. See local.go for the AES-GCM
// encrypted-at-rest backend used in local development.
package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// VaultStore is a ports.SecretStore backed by a Vault KV v2 mount. Every
// SecretRef.Path becomes a path under that mount; the raw byte payload is
// stored base64-encoded under a single "value" field, since Vault's KV
// engine only accepts JSON-serializable string values.
type VaultStore struct {
	kv *vaultapi.KVv2
}

// NewVaultStore builds a VaultStore. addr is the Vault server address
// (e.g. "https://vault.example.com:8200"); token authenticates the
// platform's own Vault identity (an AppRole or Kubernetes auth login token
// in production — a static token here is the simplest bootstrap, matching
// every other integration's single-config-value pattern); mountPath is the
// KV v2 secrets engine's mount point (commonly "secret" or "platform").
func NewVaultStore(addr, token, mountPath string) (*VaultStore, error) {
	cfg := vaultapi.DefaultConfig()
	cfg.Address = addr
	client, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating vault client: %w", err)
	}
	client.SetToken(token)
	return &VaultStore{kv: client.KVv2(mountPath)}, nil
}

func (s *VaultStore) Put(ctx context.Context, ref ports.SecretRef, value []byte) error {
	data := map[string]any{"value": base64.StdEncoding.EncodeToString(value)}
	if _, err := s.kv.Put(ctx, ref.Path, data); err != nil {
		return fmt.Errorf("writing vault secret %s: %w", ref.Path, err)
	}
	return nil
}

func (s *VaultStore) Get(ctx context.Context, ref ports.SecretRef) ([]byte, error) {
	secret, err := s.kv.Get(ctx, ref.Path)
	if err != nil {
		if isVaultNotFound(err) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("reading vault secret %s: %w", ref.Path, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, shared.ErrNotFound
	}
	encoded, ok := secret.Data["value"].(string)
	if !ok {
		return nil, fmt.Errorf("vault secret %s has no \"value\" field (was it written by a different tool?)", ref.Path)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding vault secret %s: %w", ref.Path, err)
	}
	return decoded, nil
}

func (s *VaultStore) Delete(ctx context.Context, ref ports.SecretRef) error {
	if err := s.kv.Delete(ctx, ref.Path); err != nil {
		return fmt.Errorf("deleting vault secret %s: %w", ref.Path, err)
	}
	return nil
}

// isVaultNotFound reports whether err indicates Vault returned no data for
// a path outright (as opposed to a transport/auth failure). The Vault
// client's KVv2.Get returns its own ErrSecretNotFound sentinel for a
// missing path in the common case, but a bare 404 ResponseError can surface
// too depending on the mount/proxy configuration in front of Vault.
func isVaultNotFound(err error) bool {
	if errors.Is(err, vaultapi.ErrSecretNotFound) {
		return true
	}
	var respErr *vaultapi.ResponseError
	return errors.As(err, &respErr) && respErr.StatusCode == 404
}

var _ ports.SecretStore = (*VaultStore)(nil)
