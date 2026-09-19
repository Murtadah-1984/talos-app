package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, _ := newTestClientWithServer(t)
	return c
}

// newTestClientWithServer also returns the miniredis server so tests can
// advance its virtual clock via FastForward — miniredis does not expire
// keys based on wall-clock time, so a real time.Sleep never triggers TTL
// expiry in these tests.
func newTestClientWithServer(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	return New(mr.Addr()), mr
}

func TestClient_CacheRoundTrip(t *testing.T) {
	c := newTestClient(t)
	if err := c.SetCache(t.Context(), "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("SetCache: %v", err)
	}
	got, ok, err := c.GetCache(t.Context(), "k")
	if err != nil {
		t.Fatalf("GetCache: %v", err)
	}
	if !ok || string(got) != "v" {
		t.Fatalf("expected (\"v\", true), got (%q, %v)", got, ok)
	}
}

func TestClient_GetCache_Missing(t *testing.T) {
	c := newTestClient(t)
	_, ok, err := c.GetCache(t.Context(), "missing")
	if err != nil {
		t.Fatalf("GetCache: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for a missing key")
	}
}

func TestClient_AcquireLock_SecondCallerBlocked(t *testing.T) {
	c := newTestClient(t)

	lock1, ok, err := c.AcquireLock(t.Context(), "leader", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first AcquireLock: ok=%v err=%v", ok, err)
	}
	if lock1 == nil {
		t.Fatal("expected a non-nil lock")
	}

	_, ok, err = c.AcquireLock(t.Context(), "leader", time.Minute)
	if err != nil {
		t.Fatalf("second AcquireLock: %v", err)
	}
	if ok {
		t.Fatal("expected the second acquirer to be blocked while the first holds the lock")
	}
}

func TestClient_UnlockThenReacquire(t *testing.T) {
	c := newTestClient(t)

	lock1, ok, err := c.AcquireLock(t.Context(), "leader", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireLock: ok=%v err=%v", ok, err)
	}
	if err := lock1.Unlock(t.Context()); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	_, ok, err = c.AcquireLock(t.Context(), "leader", time.Minute)
	if err != nil {
		t.Fatalf("re-AcquireLock: %v", err)
	}
	if !ok {
		t.Fatal("expected the lock to be acquirable again after Unlock")
	}
}

func TestClient_Unlock_DoesNotReleaseAnotherHoldersLock(t *testing.T) {
	c, mr := newTestClientWithServer(t)

	lock1, ok, err := c.AcquireLock(t.Context(), "leader", 20*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("AcquireLock: ok=%v err=%v", ok, err)
	}
	mr.FastForward(30 * time.Millisecond) // let it expire

	lock2, ok, err := c.AcquireLock(t.Context(), "leader", time.Minute)
	if err != nil || !ok {
		t.Fatalf("second AcquireLock after expiry: ok=%v err=%v", ok, err)
	}

	// lock1's token no longer matches what's stored (lock2 holds it now);
	// its Unlock must be a no-op, not a release of lock2's lock.
	if err := lock1.Unlock(t.Context()); err != nil {
		t.Fatalf("stale Unlock: %v", err)
	}

	_, ok, err = c.AcquireLock(t.Context(), "leader", time.Minute)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if ok {
		t.Fatal("expected lock2 to still hold the lock after lock1's stale Unlock")
	}
	_ = lock2
}

func TestLock_Renew_ExtendsHeldLock(t *testing.T) {
	c, mr := newTestClientWithServer(t)

	lock, ok, err := c.AcquireLock(t.Context(), "leader", 50*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("AcquireLock: ok=%v err=%v", ok, err)
	}

	renewed, err := lock.Renew(t.Context(), time.Minute)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if !renewed {
		t.Fatal("expected Renew to succeed for the current holder")
	}

	mr.FastForward(60 * time.Millisecond) // past the original TTL, within the renewed one
	_, ok, err = c.AcquireLock(t.Context(), "leader", time.Minute)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if ok {
		t.Fatal("expected the lock to still be held after Renew extended its TTL")
	}
}

func TestLock_Renew_FailsForLostLease(t *testing.T) {
	c, mr := newTestClientWithServer(t)

	lock, ok, err := c.AcquireLock(t.Context(), "leader", 20*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("AcquireLock: ok=%v err=%v", ok, err)
	}
	mr.FastForward(30 * time.Millisecond) // let it expire

	if _, _, err := c.AcquireLock(t.Context(), "leader", time.Minute); err != nil {
		t.Fatalf("second AcquireLock: %v", err)
	}

	renewed, err := lock.Renew(t.Context(), time.Minute)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if renewed {
		t.Fatal("expected Renew to report false once another holder has the lock")
	}
}
