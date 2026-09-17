package secrets_test

import (
	"context"
	"errors"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/infrastructure/secrets"
)

func TestLocalStore_PutGetRoundTrip(t *testing.T) {
	store, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	ctx := context.Background()
	ref := ports.SecretRef{Backend: "local", Path: "test/credential"}

	if err := store.Put(ctx, ref, []byte("super-secret-value")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "super-secret-value" {
		t.Fatalf("expected round-tripped value, got %q", got)
	}
}

func TestLocalStore_GetMissing(t *testing.T) {
	store, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	_, err = store.Get(context.Background(), ports.SecretRef{Backend: "local", Path: "does/not/exist"})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalStore_DeleteThenGetIsNotFound(t *testing.T) {
	store, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	ctx := context.Background()
	ref := ports.SecretRef{Backend: "local", Path: "to-delete"}
	if err := store.Put(ctx, ref, []byte("value")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(ctx, ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, ref); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
