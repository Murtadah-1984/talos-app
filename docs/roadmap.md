# Roadmap

Development proceeds in phases. Each phase must leave the system executable and
testable — no phase depends on unfinished work from a later phase to run.

## Phase 1 — Foundation (this delivery)

- [x] Repository structure (Clean/Hexagonal layout)
- [x] Go backend skeleton (`platform-api`, `platform-worker`, `platform-scheduler`)
- [x] PostgreSQL schema + migrations for core entities
- [x] REST API foundation (routing, middleware, OpenAPI stub)
- [x] Authentication (OIDC-ready, dev JWT issuer) + RBAC middleware
- [x] Structured logging + OpenTelemetry wiring
- [x] React/TypeScript frontend shell (routing, layout, API client)
- [x] Health/readiness endpoints
- [x] Docker Compose local dev environment
- [x] Helm charts for platform components
- [x] CI (lint, vet, test, build for Go and frontend)
- [x] Mock adapters for Talos, GitHub, Argo CD, Cluster API, Proxmox

## Phase 2 — Talos Integration

- [x] Real Talos gRPC client (`internal/integrations/talos/client.go`) behind the
      `TalosClient` port, using the official
      `github.com/siderolabs/talos/pkg/machinery/client`. Selected via
      `PLATFORM_TALOS_ADAPTER=real` (default `mock`); see
      `internal/integrations/talos/factory.go`.
- [x] Health, version, service/disk/network inspection, etcd member health — via a mix
      of the machine API (`ServiceList`, `Disks`, `EtcdStatus`, `Version`) and COSI
      resource queries (`runtime.MachineStatus`, `network.HostnameStatus`,
      `network.AddressStatus`).
- [x] Machine operations: reboot, shutdown, upgrade, config apply/read — wired through
      `machineservice` and used by the `check-talos-health` cluster-provisioning step.
- [x] Unit tests for talosconfig parsing and apply-mode mapping; a real integration
      test (`TestClient_AgainstLiveEndpoint`) gated behind `TALOS_TEST_ENDPOINT`/
      `TALOS_TEST_CONFIG` env vars, skipped in CI since no live Talos node is available
      there — run it locally against a kind/QEMU Talos node or real hardware.
- [ ] **Known limitation**: `Client` is constructed from a single talosconfig
      (`PLATFORM_TALOS_CONFIG_FILE`), covering one Talos cluster's PKI per platform
      process. `ports.TalosClient` methods are keyed only by machine endpoint, not
      cluster ID, so a real multi-cluster deployment needs per-cluster credential
      resolution threaded through — tracked as follow-up work, not yet implemented.
- [ ] Machine discovery (enumerating not-yet-known machines) — this is really an
      infrastructure-provider concern (bare metal/Proxmox `DiscoverMachines`, §10) once
      a machine already has an endpoint; nothing further to add on the Talos side.
- [ ] Cluster discovery via Talos (`DIRECT_TALOS` mode) — inferring an existing
      cluster's topology purely from querying its machines, for onboarding
      already-running clusters the platform didn't provision itself.
- [ ] Maintenance-mode (insecure, pre-PKI) connections for freshly-booted, not-yet-
      configured machines — needed once Phase 6 bare-metal/Proxmox provisioning
      workflows actually create machines and need to push their first configuration.

## Phase 3 — GitOps (GitHub)

- [ ] `GitProvider` port + GitHub implementation
- [ ] GitOps repository scaffolding generator (`gitops/clusters/...` layout)
- [ ] Manifest generation (cluster.yaml, infrastructure.yaml, control-plane.yaml, workers.yaml)
- [ ] Branch/commit/PR workflow for cluster changes
- [ ] End-to-end: cluster request → Git commit → PR → merge

## Phase 4 — Argo CD

- [ ] `ArgoCDClient` port + real client
- [ ] Application/ApplicationSet discovery, sync/health status, drift detection
- [ ] Sync triggering, rollback via Git revert, sync history

## Phase 5 — Cluster API

- [ ] `ClusterAPIProvider` port + real client (Cluster, KubeadmControlPlane,
      MachineDeployment, infra provider CRs)
- [ ] CAPI manifest generation committed via the GitOps pipeline
- [ ] Scaling, upgrades, remediation surfaced through the platform API/UI

## Phase 6 — Infrastructure Providers

- [ ] Bare Metal provider (inventory-based, BMC-agnostic core)
- [ ] Proxmox provider (VM lifecycle, cloud-init, Talos image deployment)
- [ ] Extension points documented for AWS/Azure/GCP/Equinix Metal/OpenStack/VMware

## Phase 7 — Production Hardening

- [ ] HA for API/worker/scheduler, distributed workflow locks
- [ ] Vault-backed secrets management
- [ ] Full audit coverage of destructive operations
- [ ] Backup/disaster-recovery runbooks
- [ ] Rate limiting, security hardening pass, dependency/secret scanning in CI
- [ ] Full OpenTelemetry coverage (traces/metrics/logs across every integration)

## Non-goals (always)

- Replacing Argo CD's reconciliation loop
- Replacing Cluster API's reconciliation loop
- Becoming a second source of truth for desired cluster state (Git is authoritative)
- A Kubernetes controller for every resource — only where continuous in-cluster
  reconciliation is genuinely beneficial
