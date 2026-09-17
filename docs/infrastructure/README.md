# Infrastructure Providers

See [ADR-0007](../adr/0007-infrastructure-provider-abstraction.md).

Every provider implements `ports.InfrastructureProvider`
(`internal/application/ports/infrastructure.go`):

- **Bare Metal** (`internal/integrations/baremetal`) — a real (non-mock) inventory-based
  implementation. It never fabricates hardware: `ProvisionMachine` claims an
  already-registered machine, and power control delegates to a pluggable
  `PowerController` (IPMI/Redfish land later; `NoopPowerController` reports
  `NOT_CONFIGURED` today rather than silently no-op-ing).
- **Proxmox** (`internal/integrations/proxmox`) — a mock VM lifecycle implementation
  today; the real Proxmox REST API client is Phase 6 (see [../roadmap.md](../roadmap.md)).

Adding VMware/AWS/Azure/GCP/OpenStack/Equinix Metal later means adding a new package
under `internal/integrations/` and registering it — cluster lifecycle code depends
only on the `InfrastructureProvider` interface and each provider's reported
`ProviderCapability`.
