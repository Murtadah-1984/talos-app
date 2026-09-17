// Package secrets implements the ports.SecretStore port (ADR-0006). This
// file provides the "local" backend: AES-256-GCM encryption-at-rest keyed by
// a locally-held key, intended for local development only. Production
// deployments should configure the Vault backend (added alongside this one
// behind the same interface — see docs/roadmap.md Phase 7).
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// LocalStore is an in-memory, AES-GCM-encrypted SecretStore. It is explicitly
// not durable across process restarts unless paired with a persistence layer;
// for local development that limitation is acceptable and intentional.
type LocalStore struct {
	gcm cipher.AEAD

	mu   sync.RWMutex
	data map[string][]byte
}

// NewLocalStore builds a LocalStore from a hex-encoded 32-byte key. If keyHex
// is empty, a random key is generated for the lifetime of the process (data
// will not be recoverable across restarts — fine for ephemeral dev use, never
// for production).
func NewLocalStore(keyHex string) (*LocalStore, error) {
	var key []byte
	if keyHex == "" {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generating ephemeral secret store key: %w", err)
		}
	} else {
		decoded, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("decoding PLATFORM_SECRETSTORE_KEY_HEX: %w", err)
		}
		if len(decoded) != 32 {
			return nil, fmt.Errorf("PLATFORM_SECRETSTORE_KEY_HEX must decode to 32 bytes, got %d", len(decoded))
		}
		key = decoded
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &LocalStore{gcm: gcm, data: make(map[string][]byte)}, nil
}

func refKey(ref ports.SecretRef) string {
	return ref.Backend + ":" + ref.Path
}

func (s *LocalStore) Put(_ context.Context, ref ports.SecretRef, value []byte) error {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	sealed := s.gcm.Seal(nonce, nonce, value, nil)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[refKey(ref)] = sealed
	return nil
}

func (s *LocalStore) Get(_ context.Context, ref ports.SecretRef) ([]byte, error) {
	s.mu.RLock()
	sealed, ok := s.data[refKey(ref)]
	s.mu.RUnlock()
	if !ok {
		return nil, shared.ErrNotFound
	}

	nonceSize := s.gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, fmt.Errorf("corrupt secret record for %s", ref.Path)
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	return s.gcm.Open(nil, nonce, ciphertext, nil)
}

func (s *LocalStore) Delete(_ context.Context, ref ports.SecretRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, refKey(ref))
	return nil
}
