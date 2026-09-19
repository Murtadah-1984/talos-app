package proxmox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

func newTestClient(t *testing.T, mux *http.ServeMux) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(mux)
	c := NewClient(srv.URL, "pve1", "root@pam!platform=secret", 9000)
	c.http = srv.Client()
	return c, srv.Close
}

func writeData(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func TestClient_DiscoverMachines(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve1/qemu", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeData(w, []map[string]any{
			{"vmid": 101, "name": "worker-1", "status": "running"},
			{"vmid": 102, "name": "worker-2", "status": "stopped"},
		})
	})
	c, closeFn := newTestClient(t, mux)
	defer closeFn()

	machines, err := c.DiscoverMachines(t.Context())
	if err != nil {
		t.Fatalf("DiscoverMachines: %v", err)
	}
	if len(machines) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(machines))
	}
	if gotAuth != "PVEAPIToken=root@pam!platform=secret" {
		t.Errorf("unexpected auth header: %q", gotAuth)
	}
}

func TestClient_ProvisionMachine_ClonesConfiguresAndStarts(t *testing.T) {
	var sawClone, sawConfig, sawStart bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/cluster/nextid", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, "104")
	})
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/9000/clone", func(w http.ResponseWriter, r *http.Request) {
		sawClone = true
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["newid"] != float64(104) {
			t.Errorf("expected clone to target newid 104, got %v", body["newid"])
		}
		writeData(w, "UPID:pve1:clone-task")
	})
	mux.HandleFunc("/api2/json/nodes/pve1/tasks/UPID:pve1:clone-task/status", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, map[string]string{"status": "stopped", "exitstatus": "OK"})
	})
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/104/config", func(w http.ResponseWriter, r *http.Request) {
		sawConfig = true
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT for config, got %s", r.Method)
		}
		writeData(w, nil)
	})
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/104/status/start", func(w http.ResponseWriter, r *http.Request) {
		sawStart = true
		writeData(w, "UPID:pve1:start-task")
	})

	c, closeFn := newTestClient(t, mux)
	defer closeFn()

	result, err := c.ProvisionMachine(t.Context(), ports.MachineSpec{Hostname: "worker-3", CPU: 4, MemoryBytes: 8 * 1024 * 1024 * 1024})
	if err != nil {
		t.Fatalf("ProvisionMachine: %v", err)
	}
	if result.ProviderMachineID != "104" {
		t.Errorf("expected provider machine ID 104, got %q", result.ProviderMachineID)
	}
	if !sawClone || !sawConfig || !sawStart {
		t.Errorf("expected clone, config, and start to all be called: clone=%v config=%v start=%v", sawClone, sawConfig, sawStart)
	}
}

func TestClient_ProvisionMachine_FailsOnTaskError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/cluster/nextid", func(w http.ResponseWriter, r *http.Request) { writeData(w, "104") })
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/9000/clone", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, "UPID:pve1:clone-task")
	})
	mux.HandleFunc("/api2/json/nodes/pve1/tasks/UPID:pve1:clone-task/status", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, map[string]string{"status": "stopped", "exitstatus": "clone failed: no space left"})
	})

	c, closeFn := newTestClient(t, mux)
	defer closeFn()

	if _, err := c.ProvisionMachine(t.Context(), ports.MachineSpec{Hostname: "worker-3"}); err == nil {
		t.Fatal("expected an error when the clone task fails")
	}
}

func TestClient_PowerOperations(t *testing.T) {
	var gotPaths []string
	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		writeData(w, "UPID:pve1:task")
	}
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/104/status/start", handler)
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/104/status/stop", handler)
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/104/status/reset", handler)

	c, closeFn := newTestClient(t, mux)
	defer closeFn()

	if err := c.PowerOn(t.Context(), "104"); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}
	if err := c.PowerOff(t.Context(), "104"); err != nil {
		t.Fatalf("PowerOff: %v", err)
	}
	if err := c.Reboot(t.Context(), "104"); err != nil {
		t.Fatalf("Reboot: %v", err)
	}

	want := []string{
		"/api2/json/nodes/pve1/qemu/104/status/start",
		"/api2/json/nodes/pve1/qemu/104/status/stop",
		"/api2/json/nodes/pve1/qemu/104/status/reset",
	}
	if len(gotPaths) != len(want) {
		t.Fatalf("expected %d calls, got %d: %v", len(want), len(gotPaths), gotPaths)
	}
	for i, w := range want {
		if gotPaths[i] != w {
			t.Errorf("call %d: expected %q, got %q", i, w, gotPaths[i])
		}
	}
}

func TestClient_GetMachineStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/104/status/current", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, map[string]any{"vmid": 104, "status": "running"})
	})
	c, closeFn := newTestClient(t, mux)
	defer closeFn()

	status, err := c.GetMachineStatus(t.Context(), "104")
	if err != nil {
		t.Fatalf("GetMachineStatus: %v", err)
	}
	if !status.Exists || !status.PowerOn {
		t.Errorf("expected Exists=true PowerOn=true, got %+v", status)
	}
}

func TestClient_GetMachineStatus_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/999/status/current", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	c, closeFn := newTestClient(t, mux)
	defer closeFn()

	if _, err := c.GetMachineStatus(t.Context(), "999"); err == nil {
		t.Fatal("expected an error for a nonexistent VM")
	}
}

func TestClient_DeleteMachine(t *testing.T) {
	var gotMethod string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/104", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		writeData(w, "UPID:pve1:delete-task")
	})
	c, closeFn := newTestClient(t, mux)
	defer closeFn()

	if err := c.DeleteMachine(t.Context(), "104"); err != nil {
		t.Fatalf("DeleteMachine: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
}
