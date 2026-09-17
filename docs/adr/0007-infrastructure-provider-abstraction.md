# ADR-0007: Infrastructure Provider Abstraction

## Status
Accepted

## Context
The platform must eventually support bare metal, Proxmox, VMware, OpenStack, AWS,
Azure, GCP, and Equinix Metal (§10). Coupling machine-provisioning logic to any one of
these in the domain/application layer would make adding the next provider require
touching core workflows.

## Decision
Define a single port:

```go
type InfrastructureProvider interface {
    DiscoverMachines(ctx context.Context) ([]Machine, error)
    ProvisionMachine(ctx context.Context, spec MachineSpec) (Machine, error)
    DeleteMachine(ctx context.Context, id MachineID) error
    PowerOn(ctx context.Context, id MachineID) error
    PowerOff(ctx context.Context, id MachineID) error
    Reboot(ctx context.Context, id MachineID) error
    GetMachineStatus(ctx context.Context, id MachineID) (MachineStatus, error)
    Capability() ProviderCapability
}
```

implemented independently by each provider package under
`internal/integrations/{baremetal,proxmox,...}`. A provider registry
(`internal/application/infraprovider`) resolves the correct implementation by the
`infrastructure_providers.type` column on a per-organization/per-site basis.

BMC access (IPMI/Redfish) for bare metal is a separate, narrower interface
(`PowerController`) that the bare-metal provider composes rather than something Talos
integration code depends on directly — keeping BMC concerns decoupled from Talos
concerns (§11).

Every provider reports a `ProviderCapability` (which operations it actually supports)
so the API/UI can show `NOT_SUPPORTED` instead of failing unpredictably when, e.g., a
cloud provider doesn't support a bare-metal-only operation.

## Consequences
- Phase 6 delivers Bare Metal and Proxmox only; adding VMware/AWS/etc. later means
  adding a new package + registry entry, not touching cluster lifecycle code.
- Cluster provisioning workflows call `InfrastructureProvider` generically; they don't
  branch on provider type except through `ProviderCapability` checks.
- Provider-specific credentials go through the `SecretStore` (ADR-0006); the provider
  implementation is the only code that ever decrypts and uses them.
