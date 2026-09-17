# Talos Integration

See [ADR-0001](../adr/0001-talos-integration.md).

All Talos access goes through the `ports.TalosClient` interface
(`internal/application/ports/talos.go`). Two implementations exist:

- `internal/integrations/talos.MockClient` — a deterministic in-memory stand-in, the
  default (`PLATFORM_TALOS_ADAPTER=mock`) so the rest of the platform can be built and
  tested without physical or virtual Talos machines.
- `internal/integrations/talos.Client` — the real adapter, using the official
  `github.com/siderolabs/talos/pkg/machinery/client`. Enable it with
  `PLATFORM_TALOS_ADAPTER=real` and `PLATFORM_TALOS_CONFIG_FILE=/path/to/talosconfig`.

`internal/integrations/talos/factory.go` selects between them from configuration, so
callers (`machineservice`, the cluster-provisioning workflow) depend only on the port.

## What the real client does

Every method dials directly to the target machine's endpoint (no proxying through
another node) using the talosconfig's CA/certificate/key, then either calls a
machine-API RPC directly (`Version`, `ServiceList`, `Disks`, `EtcdStatus`, `Reboot`,
`Shutdown`, `Upgrade`, `ApplyConfiguration`) or queries a COSI resource for state that
Talos exposes as a resource rather than an RPC (`runtime.MachineStatus` for readiness,
`network.HostnameStatus`/`network.AddressStatus` for identity/networking,
`config.MachineConfig` for the applied configuration YAML).

## Known limitations (tracked in [../roadmap.md](../roadmap.md))

- **Single cluster per process.** `Client` holds one talosconfig, so one platform
  deployment currently manages the PKI of one Talos cluster. `ports.TalosClient`
  methods are keyed by endpoint only; per-cluster credential resolution keyed off the
  target cluster isn't threaded through yet.
- **No maintenance-mode (insecure) connections.** Freshly-booted machines without PKI
  trust yet (used during initial bare-metal/Proxmox provisioning, Phase 6) aren't
  reachable through this client yet — only machines already possessing the cluster's
  generated certificates are.
- **Kubernetes version is not populated.** Talos's API doesn't expose it; it belongs
  to a future Kubernetes API integration, not this one.

## Testing against a real node

```bash
export TALOS_TEST_ENDPOINT=10.0.0.5
export TALOS_TEST_CONFIG=/path/to/talosconfig
go test ./internal/integrations/talos/... -run TestClient_AgainstLiveEndpoint -v
```

This test is skipped (not failed) when those variables are unset, which is why it's
safe to leave in CI.
