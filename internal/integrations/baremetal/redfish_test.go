package baremetal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/talos-platform/talos-platform/internal/infrastructure/secrets"
)

func newRedfishTestServer(t *testing.T, onReset func(resetType string)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/Systems", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Members": []map[string]string{{"@odata.id": "/redfish/v1/Systems/System.Embedded.1"}},
		})
	})
	mux.HandleFunc("/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ResetType string `json:"ResetType"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if onReset != nil {
			onReset(body.ResetType)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewTLSServer(mux)
}

func newTestRedfishController(t *testing.T, srv *httptest.Server) *RedfishController {
	t.Helper()
	store, err := secrets.NewLocalStore("")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	seedCredentials(t, store, "node-01-bmc", "admin", "s3cr3t")
	c := NewRedfishController(store)
	c.http = srv.Client()
	return c
}

// bmcAddressFor strips the https:// scheme from a test server's URL, since
// PowerController methods take a bare host:port and the controller always
// prepends "https://" itself.
func bmcAddressFor(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "https://")
}

func TestRedfishController_PowerOn_SendsResetTypeOn(t *testing.T) {
	var gotResetType string
	srv := newRedfishTestServer(t, func(resetType string) { gotResetType = resetType })
	defer srv.Close()

	c := newTestRedfishController(t, srv)
	if err := c.PowerOn(t.Context(), bmcAddressFor(srv), "node-01-bmc"); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}
	if gotResetType != "On" {
		t.Errorf("expected ResetType On, got %q", gotResetType)
	}
}

func TestRedfishController_PowerOff_SendsForceOff(t *testing.T) {
	var gotResetType string
	srv := newRedfishTestServer(t, func(resetType string) { gotResetType = resetType })
	defer srv.Close()

	c := newTestRedfishController(t, srv)
	if err := c.PowerOff(t.Context(), bmcAddressFor(srv), "node-01-bmc"); err != nil {
		t.Fatalf("PowerOff: %v", err)
	}
	if gotResetType != "ForceOff" {
		t.Errorf("expected ResetType ForceOff, got %q", gotResetType)
	}
}

func TestRedfishController_Reboot_SendsForceRestart(t *testing.T) {
	var gotResetType string
	srv := newRedfishTestServer(t, func(resetType string) { gotResetType = resetType })
	defer srv.Close()

	c := newTestRedfishController(t, srv)
	if err := c.Reboot(t.Context(), bmcAddressFor(srv), "node-01-bmc"); err != nil {
		t.Fatalf("Reboot: %v", err)
	}
	if gotResetType != "ForceRestart" {
		t.Errorf("expected ResetType ForceRestart, got %q", gotResetType)
	}
}

func TestRedfishController_NoSystemsFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/Systems", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Members": []map[string]string{}})
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := newTestRedfishController(t, srv)
	if err := c.PowerOn(t.Context(), bmcAddressFor(srv), "node-01-bmc"); err == nil {
		t.Fatal("expected an error when no ComputerSystem is found")
	}
}
