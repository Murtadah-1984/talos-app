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

- [ ] Real Talos gRPC client (`internal/integrations/talos`) behind the `TalosClient` port
- [ ] Machine discovery, health, version, service/disk/network inspection
- [ ] Machine operations: reboot, shutdown, upgrade, config apply
- [ ] Cluster discovery via Talos (`DIRECT_TALOS` mode)
- [ ] Integration tests against a real or emulated Talos endpoint

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
