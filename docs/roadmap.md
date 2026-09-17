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

- [x] `GitProvider` port + real GitHub implementation
      (`internal/integrations/github/client.go`), using `go-github` and the Git Data
      API so a whole manifest set commits atomically (branch create, blob-inlined tree,
      commit, ref update, PR create/get/merge). Selected via
      `PLATFORM_GITHUB_ADAPTER=real` + `PLATFORM_GITHUB_TOKEN` (default `mock`); see
      `internal/integrations/github/factory.go`.
- [x] Manifest generation (`internal/application/gitopsrender`): pure, deterministic
      rendering of `cluster.yaml`, `infrastructure.yaml`, `control-plane.yaml`,
      `workers.yaml`, and (when Argo CD is enabled) `argocd-application.yaml` — the
      `gitops/clusters/<name>/...` layout from §3. Deliberately no Kustomize, per §3's
      own preference for Helm.
- [x] Branch/commit/PR workflow for cluster changes — wired into the
      `commit-to-git`/`create-pull-request`/`await-and-merge-pull-request` steps of
      `NewClusterProvisionDefinition`, including CAPI manifests when the cluster's
      provider mode is `CLUSTER_API`. Fixed a Phase-1 bug in the process: the commit
      and PR steps had computed two different, non-matching branch names.
- [x] `gitops.ChangeSet` now tracks real status transitions (`PR_CREATED` ->
      `MERGED` -> `SYNCED`) instead of being created once and left stale.
- [x] End-to-end test (`TestClusterProvisionWorkflow_EndToEnd`) exercising the full
      cluster request → Git commit → PR → merge → Argo CD sync → READY pipeline
      against the mock adapters.
- [ ] **GitOps repository scaffolding generator** — bootstrapping a brand-new `gitops/`
      repository's top-level layout (`infrastructure/`, `applications/`, App-of-Apps
      root Applications per §3–§4) is not yet built; today the renderer only produces
      one cluster's `clusters/<name>/` subtree, assuming the repository and its
      Argo CD App-of-Apps root already exist.
- [ ] The human-in-the-loop approval gate (§4) is not yet webhook-driven — the
      `await-and-merge-pull-request` step merges immediately rather than pausing for a
      GitHub PR-merged webhook. Needed before this pipeline is safe for
      review-required production changes.

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
