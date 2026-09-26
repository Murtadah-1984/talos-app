package talos

import (
	"context"
	"os"
	"testing"

	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

const testTalosconfig = `
context: default
contexts:
  default:
    endpoints:
      - 10.0.0.1
    ca: dGVzdC1jYQ==
    crt: dGVzdC1jcnQ=
    key: dGVzdC1rZXk=
`

func TestNewClient_ParsesValidTalosconfig(t *testing.T) {
	c, err := NewClient([]byte(testTalosconfig))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Capability() != shared.CapabilityAvailable {
		t.Fatalf("expected AVAILABLE capability once constructed, got %s", c.Capability())
	}
}

func TestNewClient_RejectsInvalidYAML(t *testing.T) {
	_, err := NewClient([]byte("not: valid: talosconfig: [["))
	if err == nil {
		t.Fatal("expected an error parsing invalid talosconfig YAML")
	}
}

const otherClusterTalosconfig = `
context: other
contexts:
  other:
    endpoints:
      - 10.1.0.1
    ca: b3RoZXItY2E=
    crt: b3RoZXItY3J0
    key: b3RoZXIta2V5
`

func TestClient_ConfigFor_FallsBackToDefault(t *testing.T) {
	c, err := NewClient([]byte(testTalosconfig))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cfg, err := c.configFor("10.0.0.1")
	if err != nil {
		t.Fatalf("configFor: %v", err)
	}
	if cfg != c.cfg {
		t.Error("expected an unregistered endpoint to resolve to the default talosconfig")
	}
}

func TestClient_ConfigFor_PrefersRegisteredCredentials(t *testing.T) {
	c, err := NewClient([]byte(testTalosconfig))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.EnsureCredentials(context.Background(), "10.1.0.5", []byte(otherClusterTalosconfig)); err != nil {
		t.Fatalf("EnsureCredentials: %v", err)
	}

	registered, err := c.configFor("10.1.0.5")
	if err != nil {
		t.Fatalf("configFor(registered): %v", err)
	}
	if registered == c.cfg {
		t.Error("expected the registered endpoint to resolve to its own config, not the default")
	}

	fallback, err := c.configFor("10.0.0.1")
	if err != nil {
		t.Fatalf("configFor(unregistered): %v", err)
	}
	if fallback != c.cfg {
		t.Error("expected an unregistered endpoint to still resolve to the default talosconfig")
	}
}

func TestClient_ConfigFor_NoDefaultAndNoRegistration_Errors(t *testing.T) {
	c := &Client{} // no default config, nothing registered
	if _, err := c.configFor("10.0.0.1"); err == nil {
		t.Fatal("expected an error when no default config and no registration exist for the endpoint")
	}
}

func TestApplyModeFor(t *testing.T) {
	tests := map[string]machineapi.ApplyConfigurationRequest_Mode{
		"no-reboot": machineapi.ApplyConfigurationRequest_NO_REBOOT,
		"staged":    machineapi.ApplyConfigurationRequest_STAGED,
		"":          machineapi.ApplyConfigurationRequest_AUTO,
		"reboot":    machineapi.ApplyConfigurationRequest_AUTO,
	}
	for mode, want := range tests {
		if got := applyModeFor(mode); got != want {
			t.Errorf("applyModeFor(%q) = %v, want %v", mode, got, want)
		}
	}
}

func TestMaintenanceFingerprints(t *testing.T) {
	if got := maintenanceFingerprints(""); got != nil {
		t.Errorf("expected nil fingerprints for an empty string, got %v", got)
	}
	got := maintenanceFingerprints("AA:BB:CC")
	if len(got) != 1 || got[0] != "AA:BB:CC" {
		t.Errorf("unexpected fingerprints: %v", got)
	}
}

// TestClient_AgainstLiveEndpoint is a real integration test against a
// running Talos node, per the Phase 2 roadmap item "integration tests
// against a real or emulated Talos endpoint". It is skipped unless
// TALOS_TEST_ENDPOINT and TALOS_TEST_CONFIG (a talosconfig file path) are
// both set, since no such endpoint is available in this repository's CI —
// run it locally against a kind/QEMU Talos node or real hardware.
func TestClient_AgainstLiveEndpoint(t *testing.T) {
	endpoint := os.Getenv("TALOS_TEST_ENDPOINT")
	configPath := os.Getenv("TALOS_TEST_CONFIG")
	if endpoint == "" || configPath == "" {
		t.Skip("set TALOS_TEST_ENDPOINT and TALOS_TEST_CONFIG to run this test against a real Talos node")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading TALOS_TEST_CONFIG: %v", err)
	}
	client, err := NewClient(data)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()

	version, err := client.GetVersion(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if version.TalosVersion == "" {
		t.Error("expected a non-empty Talos version")
	}

	status, err := client.GetMachineStatus(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetMachineStatus: %v", err)
	}
	if status.Hostname == "" {
		t.Error("expected a non-empty hostname")
	}

	if _, err := client.GetServices(ctx, endpoint); err != nil {
		t.Errorf("GetServices: %v", err)
	}
	if _, err := client.GetDisks(ctx, endpoint); err != nil {
		t.Errorf("GetDisks: %v", err)
	}
	if _, err := client.GetNetworkInfo(ctx, endpoint); err != nil {
		t.Errorf("GetNetworkInfo: %v", err)
	}
}
