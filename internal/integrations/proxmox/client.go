// Package proxmox implements the ports.InfrastructureProvider adapter for
// Proxmox VE (§12, ADR-0007). This file is the Phase 6 real implementation:
// a hand-rolled REST client against the Proxmox API (JSON over HTTPS,
// API-token authenticated) rather than a third-party SDK, since the
// operations needed here (clone a template, configure, start/stop/reset,
// delete, query status) are a small, stable slice of Proxmox's API. See
// mock.go for the deterministic stand-in used elsewhere in local
// development.
//
// Talos image/template deployment (§12): a machine is provisioned by
// cloning a pre-built Talos VM template (its VMID configured via
// TemplateVMID) rather than installing Talos from scratch on each VM — the
// standard fast-provisioning pattern for Proxmox. cloud-init fields
// (ports.MachineSpec.CloudInit) are written to the clone's cicustom/
// ciuser config for network/user-data injection where the template
// supports it; Talos machines that instead expect their config via
// metadata server or `talos.config=` kernel argument should leave
// CloudInit empty and rely on the platform's own machine-configuration
// application (ADR-0001) once the VM boots.
package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Client is the real ports.InfrastructureProvider adapter for one Proxmox
// node.
type Client struct {
	baseURL      string
	node         string
	apiToken     string // "user@realm!tokenid=secret", as Proxmox itself formats it
	templateVMID int
	http         *http.Client
}

// NewClient builds a Client. baseURL is the Proxmox API root (e.g.
// "https://pve.example.com:8006"); node is the Proxmox node new VMs are
// created on; apiToken is a pre-formatted Proxmox API token
// ("user@realm!tokenid=secret", as `pveum user token add` prints it);
// templateVMID is the VMID of a pre-built Talos VM template to clone.
func NewClient(baseURL, node, apiToken string, templateVMID int) *Client {
	return &Client{
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		node:         node,
		apiToken:     apiToken,
		templateVMID: templateVMID,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

type proxmoxEnvelope[T any] struct {
	Data T `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling proxmox api %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return shared.ErrNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("proxmox api %s %s returned %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type qemuStatus struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (c *Client) DiscoverMachines(ctx context.Context) ([]ports.ProvisionedMachine, error) {
	var resp proxmoxEnvelope[[]qemuStatus]
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api2/json/nodes/%s/qemu", c.node), nil, &resp); err != nil {
		return nil, fmt.Errorf("listing VMs on node %s: %w", c.node, err)
	}
	out := make([]ports.ProvisionedMachine, 0, len(resp.Data))
	for _, vm := range resp.Data {
		out = append(out, ports.ProvisionedMachine{ProviderMachineID: strconv.Itoa(vm.VMID)})
	}
	return out, nil
}

// ProvisionMachine clones the configured Talos template into a new VM,
// applies spec's resource sizing, and starts it. The clone is a Proxmox
// background task; this call blocks until it completes or ctx is done.
func (c *Client) ProvisionMachine(ctx context.Context, spec ports.MachineSpec) (ports.ProvisionedMachine, error) {
	var nextID proxmoxEnvelope[string]
	if err := c.do(ctx, http.MethodGet, "/api2/json/cluster/nextid", nil, &nextID); err != nil {
		return ports.ProvisionedMachine{}, fmt.Errorf("allocating VMID: %w", err)
	}
	newID, err := strconv.Atoi(nextID.Data)
	if err != nil {
		return ports.ProvisionedMachine{}, fmt.Errorf("parsing allocated VMID %q: %w", nextID.Data, err)
	}

	var cloneTask proxmoxEnvelope[string]
	cloneReq := map[string]any{"newid": newID, "name": spec.Hostname, "full": 1}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/clone", c.node, c.templateVMID), cloneReq, &cloneTask); err != nil {
		return ports.ProvisionedMachine{}, fmt.Errorf("cloning template %d: %w", c.templateVMID, err)
	}
	if err := c.waitForTask(ctx, cloneTask.Data); err != nil {
		return ports.ProvisionedMachine{}, fmt.Errorf("waiting for clone of %d to %d: %w", c.templateVMID, newID, err)
	}

	cores := spec.CPU
	if cores <= 0 {
		cores = 1
	}
	memoryMB := spec.MemoryBytes / (1024 * 1024)
	configReq := map[string]any{"cores": cores, "memory": memoryMB}
	if spec.CloudInit != "" {
		configReq["cicustom"] = spec.CloudInit
	}
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", c.node, newID), configReq, nil); err != nil {
		return ports.ProvisionedMachine{}, fmt.Errorf("configuring VM %d: %w", newID, err)
	}

	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/start", c.node, newID), nil, nil); err != nil {
		return ports.ProvisionedMachine{}, fmt.Errorf("starting VM %d: %w", newID, err)
	}

	// The management IP isn't reliably known immediately after boot
	// (DHCP hasn't necessarily leased yet, and the qemu-guest-agent isn't
	// installed on a minimal Talos image) — left blank rather than
	// guessed; machine discovery/reconciliation resolves it once the
	// platform can reach the node directly.
	return ports.ProvisionedMachine{ProviderMachineID: strconv.Itoa(newID)}, nil
}

// waitForTask polls a Proxmox background task (identified by its UPID)
// until it stops, per Proxmox's task-status API.
func (c *Client) waitForTask(ctx context.Context, upid string) error {
	type taskStatus struct {
		Status     string `json:"status"`
		ExitStatus string `json:"exitstatus"`
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		var resp proxmoxEnvelope[taskStatus]
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api2/json/nodes/%s/tasks/%s/status", c.node, upid), nil, &resp); err != nil {
			return fmt.Errorf("checking task %s: %w", upid, err)
		}
		if resp.Data.Status == "stopped" {
			if resp.Data.ExitStatus != "OK" {
				return fmt.Errorf("task %s finished with status %q", upid, resp.Data.ExitStatus)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) DeleteMachine(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api2/json/nodes/%s/qemu/%s", c.node, id), nil, nil); err != nil {
		return fmt.Errorf("deleting VM %s: %w", id, err)
	}
	return nil
}

func (c *Client) PowerOn(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%s/status/start", c.node, id), nil, nil); err != nil {
		return fmt.Errorf("starting VM %s: %w", id, err)
	}
	return nil
}

// PowerOff is a hard power-off (Proxmox "stop", equivalent to pulling the
// plug), matching the PowerController contract's semantics elsewhere
// (IPMI chassis power off, Redfish ForceOff) rather than a graceful ACPI
// shutdown.
func (c *Client) PowerOff(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%s/status/stop", c.node, id), nil, nil); err != nil {
		return fmt.Errorf("stopping VM %s: %w", id, err)
	}
	return nil
}

// Reboot is a hard reset (Proxmox "reset"), matching IPMI's "chassis power
// cycle" and Redfish's "ForceRestart".
func (c *Client) Reboot(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%s/status/reset", c.node, id), nil, nil); err != nil {
		return fmt.Errorf("resetting VM %s: %w", id, err)
	}
	return nil
}

func (c *Client) GetMachineStatus(ctx context.Context, id string) (ports.MachineStatusInfra, error) {
	var resp proxmoxEnvelope[qemuStatus]
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api2/json/nodes/%s/qemu/%s/status/current", c.node, id), nil, &resp); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return ports.MachineStatusInfra{}, shared.ErrNotFound
		}
		return ports.MachineStatusInfra{}, fmt.Errorf("getting status of VM %s: %w", id, err)
	}
	return ports.MachineStatusInfra{Exists: true, PowerOn: resp.Data.Status == "running"}, nil
}

func (c *Client) Capability() ports.ProviderCapability {
	return ports.ProviderCapability{
		State:                shared.CapabilityAvailable,
		SupportsDiscovery:    true,
		SupportsProvision:    true,
		SupportsPowerControl: true,
	}
}

var _ ports.InfrastructureProvider = (*Client)(nil)
