package baremetal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// RedfishController implements PowerController against the standard DMTF
// Redfish API (Dell iDRAC, HPE iLO, Supermicro, and most modern server BMCs
// speak it), via a hand-rolled REST client rather than a full Redfish SDK —
// the platform only ever needs the ComputerSystem.Reset action.
type RedfishController struct {
	secrets ports.SecretStore
	http    *http.Client
}

func NewRedfishController(secrets ports.SecretStore) *RedfishController {
	return &RedfishController{secrets: secrets, http: &http.Client{Timeout: 30 * time.Second}}
}

type redfishCollection struct {
	Members []struct {
		ODataID string `json:"@odata.id"`
	} `json:"Members"`
}

// systemPath discovers the first ComputerSystem's @odata.id under
// /redfish/v1/Systems. Most servers expose exactly one system per BMC.
func (c *RedfishController) systemPath(ctx context.Context, baseURL string, creds bmcCredentials) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/redfish/v1/Systems", nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(creds.Username, creds.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("listing Redfish systems: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("listing Redfish systems returned %d", resp.StatusCode)
	}

	var collection redfishCollection
	if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
		return "", fmt.Errorf("decoding Redfish systems collection: %w", err)
	}
	if len(collection.Members) == 0 {
		return "", fmt.Errorf("no ComputerSystem found under %s/redfish/v1/Systems", baseURL)
	}
	return collection.Members[0].ODataID, nil
}

func (c *RedfishController) reset(ctx context.Context, bmcAddress, credentialRef, resetType string) error {
	creds, err := resolveCredentials(ctx, c.secrets, credentialRef)
	if err != nil {
		return err
	}
	baseURL := "https://" + bmcAddress

	systemPath, err := c.systemPath(ctx, baseURL, creds)
	if err != nil {
		return err
	}

	body, err := json.Marshal(map[string]string{"ResetType": resetType})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+systemPath+"/Actions/ComputerSystem.Reset", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(creds.Username, creds.Password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("resetting %s (%s): %w", bmcAddress, resetType, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("resetting %s (%s) returned %d", bmcAddress, resetType, resp.StatusCode)
	}
	return nil
}

func (c *RedfishController) PowerOn(ctx context.Context, bmcAddress, credentialRef string) error {
	return c.reset(ctx, bmcAddress, credentialRef, "On")
}

func (c *RedfishController) PowerOff(ctx context.Context, bmcAddress, credentialRef string) error {
	return c.reset(ctx, bmcAddress, credentialRef, "ForceOff")
}

func (c *RedfishController) Reboot(ctx context.Context, bmcAddress, credentialRef string) error {
	return c.reset(ctx, bmcAddress, credentialRef, "ForceRestart")
}

func (c *RedfishController) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}

var _ PowerController = (*RedfishController)(nil)
