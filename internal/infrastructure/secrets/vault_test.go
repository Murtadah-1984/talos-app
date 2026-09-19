package secrets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// newFakeVaultServer emulates just enough of Vault's KV v2 HTTP API
// (https://developer.hashicorp.com/vault/api-docs/secret/kv/kv-v2) to
// exercise VaultStore without a real `vault` server binary.
func newFakeVaultServer(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	store := map[string]map[string]any{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/platform/data/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/v1/platform/data/"):]
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodPost, http.MethodPut:
			var body struct {
				Data map[string]any `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			store[path] = body.Data
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"version": 1, "created_time": "2026-01-01T00:00:00Z"},
			})
		case http.MethodGet:
			data, ok := store[path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{}})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"data": data, "metadata": map[string]any{"version": 1}},
			})
		case http.MethodDelete:
			delete(store, path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return httptest.NewServer(mux)
}

func TestVaultStore_PutGetRoundTrip(t *testing.T) {
	srv := newFakeVaultServer(t)
	defer srv.Close()

	store, err := NewVaultStore(srv.URL, "dev-token", "platform")
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}

	ref := ports.SecretRef{Backend: "vault", Path: "talos/cluster-1"}
	if err := store.Put(t.Context(), ref, []byte("super-secret-value")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(t.Context(), ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "super-secret-value" {
		t.Fatalf("expected round-tripped value, got %q", got)
	}
}

func TestVaultStore_GetMissing(t *testing.T) {
	srv := newFakeVaultServer(t)
	defer srv.Close()

	store, err := NewVaultStore(srv.URL, "dev-token", "platform")
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}
	_, err = store.Get(t.Context(), ports.SecretRef{Backend: "vault", Path: "does/not/exist"})
	if err == nil {
		t.Fatal("expected an error for a missing secret")
	}
}

func TestVaultStore_DeleteThenGetIsNotFound(t *testing.T) {
	srv := newFakeVaultServer(t)
	defer srv.Close()

	store, err := NewVaultStore(srv.URL, "dev-token", "platform")
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}
	ref := ports.SecretRef{Backend: "vault", Path: "to-delete"}
	if err := store.Put(t.Context(), ref, []byte("value")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(t.Context(), ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(t.Context(), ref); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestVaultStore_StoresValueBase64Encoded(t *testing.T) {
	// Sanity check on the wire format: confirms Put actually sends a
	// base64 string (Vault's KV engine cannot store raw binary values
	// directly since it's JSON), rather than relying solely on the
	// round-trip test to catch an encoding regression.
	var capturedValue string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/platform/data/", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedValue, _ = body.Data["value"].(string)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"version": 1}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	store, err := NewVaultStore(srv.URL, "dev-token", "platform")
	if err != nil {
		t.Fatalf("NewVaultStore: %v", err)
	}
	if err := store.Put(t.Context(), ports.SecretRef{Path: "x"}, []byte("binary\x00data")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(capturedValue)
	if err != nil {
		t.Fatalf("expected a valid base64 payload on the wire, got %q: %v", capturedValue, err)
	}
	if string(decoded) != "binary\x00data" {
		t.Errorf("decoded payload mismatch: %q", decoded)
	}
}
