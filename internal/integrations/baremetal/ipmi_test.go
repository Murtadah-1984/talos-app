package baremetal

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/infrastructure/secrets"
)

func seedCredentials(t *testing.T, store ports.SecretStore, ref, username, password string) {
	t.Helper()
	data, err := json.Marshal(bmcCredentials{Username: username, Password: password})
	if err != nil {
		t.Fatalf("marshaling credentials: %v", err)
	}
	if err := store.Put(context.Background(), ports.SecretRef{Backend: "baremetal", Path: ref}, data); err != nil {
		t.Fatalf("seeding credentials: %v", err)
	}
}

type fakeRunner struct {
	gotName string
	gotEnv  []string
	gotArgs []string
	err     error
}

func (f *fakeRunner) Run(_ context.Context, name string, env []string, args ...string) ([]byte, error) {
	f.gotName = name
	f.gotEnv = env
	f.gotArgs = args
	return []byte("ok"), f.err
}

func TestIPMIController_PowerOn_InvokesIpmitoolWithPasswordViaEnv(t *testing.T) {
	store, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	seedCredentials(t, store, "node-01-bmc", "admin", "s3cr3t")

	runner := &fakeRunner{}
	controller := &IPMIController{secrets: store, runner: runner}

	if err := controller.PowerOn(context.Background(), "10.0.0.5", "node-01-bmc"); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}

	if runner.gotName != "ipmitool" {
		t.Errorf("expected ipmitool to be invoked, got %q", runner.gotName)
	}
	foundPasswordEnv := false
	for _, e := range runner.gotEnv {
		if e == "IPMI_PASSWORD=s3cr3t" {
			foundPasswordEnv = true
		}
	}
	if !foundPasswordEnv {
		t.Errorf("expected IPMI_PASSWORD in env, got %v", runner.gotEnv)
	}
	for _, arg := range runner.gotArgs {
		if arg == "s3cr3t" {
			t.Fatal("password must never appear as a plain command-line argument")
		}
	}
	wantArgs := []string{"-I", "lanplus", "-H", "10.0.0.5", "-U", "admin", "-E", "chassis", "power", "on"}
	if len(runner.gotArgs) != len(wantArgs) {
		t.Fatalf("unexpected args: %v", runner.gotArgs)
	}
	for i, want := range wantArgs {
		if runner.gotArgs[i] != want {
			t.Errorf("arg %d: expected %q, got %q", i, want, runner.gotArgs[i])
		}
	}
}

func TestIPMIController_Reboot_UsesChassisPowerCycle(t *testing.T) {
	store, _ := secrets.NewLocalStore("")
	seedCredentials(t, store, "ref", "admin", "pw")
	runner := &fakeRunner{}
	controller := &IPMIController{secrets: store, runner: runner}

	if err := controller.Reboot(context.Background(), "10.0.0.5", "ref"); err != nil {
		t.Fatalf("Reboot: %v", err)
	}
	if len(runner.gotArgs) == 0 || runner.gotArgs[len(runner.gotArgs)-1] != "cycle" {
		t.Errorf("expected chassis power cycle, got args %v", runner.gotArgs)
	}
}

func TestIPMIController_PropagatesCommandFailure(t *testing.T) {
	store, _ := secrets.NewLocalStore("")
	seedCredentials(t, store, "ref", "admin", "pw")
	runner := &fakeRunner{err: errors.New("boom")}
	controller := &IPMIController{secrets: store, runner: runner}

	if err := controller.PowerOff(context.Background(), "10.0.0.5", "ref"); err == nil {
		t.Fatal("expected an error when the underlying command fails")
	}
}

func TestIPMIController_MissingCredentialsFail(t *testing.T) {
	store, _ := secrets.NewLocalStore("")
	controller := NewIPMIController(store)
	if err := controller.PowerOn(context.Background(), "10.0.0.5", "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unresolvable credential reference")
	}
}
