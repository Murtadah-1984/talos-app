# Talos Integration

See [ADR-0001](../adr/0001-talos-integration.md).

All Talos access goes through the `ports.TalosClient` interface
(`internal/application/ports/talos.go`). Today it is backed by
`internal/integrations/talos.MockClient` — a deterministic in-memory stand-in so the
rest of the platform (machine health/reboot/upgrade, cluster provisioning workflows)
can be built and tested without physical or virtual Talos machines.

The real gRPC-based client (using `github.com/siderolabs/talos/pkg/machinery/client`)
is Phase 2 work — see [../roadmap.md](../roadmap.md). Nothing above the
`TalosClient` interface will need to change when it lands.
