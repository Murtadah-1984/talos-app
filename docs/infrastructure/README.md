# Infrastructure Providers

See [ADR-0007](../adr/0007-infrastructure-provider-abstraction.md).

Every provider implements `ports.InfrastructureProvider`
(`internal/application/ports/infrastructure.go`) and is resolved at runtime by
`internal/application/inframanager.Registry`, keyed on `infraprovider.Type`.

## What's real today

- **Bare Metal** (`internal/integrations/baremetal`) — never fabricates hardware:
  `ProvisionMachine` claims an already-registered machine. Power control dispatches
  per-machine to whichever `PowerController` matches that machine's own declared BMC
  protocol:
  - `IPMIController` (`ipmi.go`) shells out to `ipmitool` (there is no mature pure-Go
    IPMI stack worth vendoring for this narrow use). The BMC password is passed via
    the `IPMI_PASSWORD` environment variable and `-E`, never as a plain CLI argument.
  - `RedfishController` (`redfish.go`) is a hand-rolled REST client against the
    standard DMTF Redfish `ComputerSystem.Reset` action (works against Dell iDRAC, HPE
    iLO, Supermicro, and most modern server BMCs).
  - A protocol with no controller enabled reports `NOT_CONFIGURED`
    (`NoopPowerController`) rather than silently no-op-ing.
  - Enable via `PLATFORM_BAREMETAL_IPMI_ENABLED` / `PLATFORM_BAREMETAL_REDFISH_ENABLED`
    — both may be on at once for a mixed-protocol inventory.
- **Proxmox** (`internal/integrations/proxmox`) — a hand-rolled REST client against the
  Proxmox API (API-token authenticated): VM discovery, cloning a pre-built Talos VM
  template (§12's "Talos image/template deployment"), resource sizing, start/stop/
  reset, deletion, and status polling. Enable via `PLATFORM_PROXMOX_ADAPTER=real` +
  `PLATFORM_PROXMOX_API_URL`/`PLATFORM_PROXMOX_NODE`/`PLATFORM_PROXMOX_API_TOKEN`/
  `PLATFORM_PROXMOX_TEMPLATE_VMID`.

Both fall back to their in-memory mocks by default (`PLATFORM_PROXMOX_ADAPTER=mock`;
bare metal's controllers default to `NoopPowerController` until explicitly enabled).

## Hard power control vs. Talos-level operations

`POST /machines/{id}/power/{on,off,cycle}` (and `talos-platform machine
power-{on,off,cycle}`) go through the machine's `InfrastructureProvider` — a BMC or
hypervisor API — and work even when the OS itself is unresponsive. This is distinct
from `POST /machines/{id}/reboot`, which asks Talos to reboot gracefully via
`ports.TalosClient` (ADR-0001) and does nothing useful if Talos isn't answering.

## Known limitation

Like every other Phase 2-5 adapter, the real Proxmox client and bare-metal power
controllers are each one process-wide instance built from global configuration — not a
distinct instance per organization's own registered `infraprovider.InfrastructureProvider`
row and credentials. Multi-tenant credential resolution is tracked in
[../roadmap.md](../roadmap.md) as follow-up work across every integration, not
special-cased here.

## Adding a new provider (AWS, Azure, GCP, OpenStack, VMware, Equinix Metal, ...)

1. Add the `infraprovider.Type` constant if it doesn't already exist
   (`internal/domain/infraprovider/infraprovider.go`).
2. Create `internal/integrations/<provider>/` with a `Client` implementing
   `ports.InfrastructureProvider` (`DiscoverMachines`, `ProvisionMachine`,
   `DeleteMachine`, `PowerOn`, `PowerOff`, `Reboot`, `GetMachineStatus`, `Capability`).
   Start with a `mock.go` (see `proxmox/mock.go` for the pattern) so the rest of the
   platform can be built and tested against it before the real client exists.
3. Add a `factory.go` with `NewFromConfig(adapterMode string, ...) (ports.InfrastructureProvider, error)`
   selecting mock vs. real, matching every other integration's pattern.
4. Add the provider's config fields to `internal/infrastructure/config/config.go` and
   wire the factory call into `cmd/platform-api/main.go`'s `inframanager.Registry`
   construction.
5. Report `ProviderCapability` honestly — set `SupportsProvision`/
  `SupportsPowerControl`/`SupportsDiscovery` to what the provider actually supports,
   so the UI/API can show `NOT_SUPPORTED` instead of guessing.
6. Cluster lifecycle code never needs to change: it depends only on
   `ports.InfrastructureProvider` and the registry, never on a specific provider type.
