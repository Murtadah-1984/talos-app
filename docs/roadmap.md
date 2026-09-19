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

- [x] `ArgoCDClient` port + real client (`internal/integrations/argocd/client.go`) — a
      hand-rolled REST client against Argo CD's documented HTTP API
      (`/api/v1/applications`), deliberately not the official Argo CD Go SDK (which
      pulls in most of k8s client-go for a read-mostly status/sync client). Selected via
      `PLATFORM_ARGOCD_ADAPTER=real` + `PLATFORM_ARGOCD_SERVER_URL` +
      `PLATFORM_ARGOCD_TOKEN` (default `mock`).
- [x] Application discovery, sync/health status, sync history — `ListApplications`,
      `GetApplication`, `GetHistory`.
- [x] Drift/health detection wired into `platform-scheduler`'s reconciliation loop: for
      every Argo-CD-enabled cluster, it now calls `GetApplication`, refreshes the
      `gitops.ArgoApplication` cache the cluster GitOps tab reads, and fires/resolves
      `audit.Alert`s ("GitOps drift detected", "Argo CD application unhealthy") — a
      pure visibility pass, per ADR-0003.
- [x] Sync triggering — `POST /api/v1/clusters/{id}/sync` (`ClusterService.TriggerSync`),
      the CLI's `talos-platform gitops sync`, and a "Trigger Sync Now" button on the
      cluster GitOps tab. This is the one place `clusterservice` calls an integration
      port directly rather than going through a workflow — deliberately, since it's an
      imperative "sync now" request, not a desired-state change (mirrors how
      `machineservice` calls `TalosClient` directly for reboot/upgrade).
- [x] Fixed a real bug found while wiring this up: the `alerts` table's unique
      constraint included `status`, so a FIRING -> RESOLVED transition inserted a
      second row instead of updating the alert in place, leaving stale FIRING rows
      forever. Migration `0005` corrects the constraint to `(target_kind, target_id,
      title)`, and `UpsertAlert` now updates `fired_at`/`resolved_at` correctly across
      re-fire/resolve cycles.
- [ ] **ApplicationSet discovery** is not implemented — only individual Applications.
- [ ] **Rollback via Git revert** (§18, §20) is not implemented. A correct
      implementation needs the commit SHA a change produced (not currently persisted
      on `gitops.ChangeSet`) and a way to resolve a commit's parent tree, then
      re-commit the reverted file contents as a new change — real work, deliberately
      not force-fit into this pass. Tracked as a follow-up, not faked.

## Phase 5 — Cluster API

- [x] `ClusterAPIProvider` port + real client (`internal/integrations/clusterapi/client.go`).
      `RenderManifests` is pure manifest generation (no client needed) for the core CAPI
      `Cluster`/`MachineDeployment` resources (stable upstream schema) plus
      `TalosControlPlane`/`TalosConfigTemplate` from Sidero Labs' Cluster API Control
      Plane/Bootstrap Providers for Talos, rendered best-effort — those two CRDs are
      less universally documented than core CAPI, flagged in-code as "verify against
      your installed CACPPT/CABPT version." `GetClusterStatus` is a real, read-only
      `k8s.io/client-go` dynamic client against the management cluster (never writes
      CAPI resources — ADR-0002). Selected via `PLATFORM_CLUSTERAPI_ADAPTER=real` +
      `PLATFORM_CLUSTERAPI_KUBECONFIG_FILE` (default `mock`).
- [x] CAPI manifest generation committed via the GitOps pipeline — not just at
      provisioning time: `internal/workflows/capi_change.go` adds
      `commitClusterAPIChange`/`waitForClusterAPIReady`, shared by provisioning,
      `CLUSTER_UPGRADE`, and `WORKER_SCALE`, so a `CLUSTER_API`-mode cluster's upgrades
      and scale requests also commit -> PR -> merge -> (Argo CD sync if enabled) rather
      than being silent no-ops. `DIRECT_TALOS`-mode clusters skip this path entirely and
      keep using `TalosClient` directly.
- [x] Scaling/upgrades surfaced through the platform API/UI — already-existing
      `POST /clusters/{id}/upgrade` and `/scale` now actually reach Cluster API for
      `CLUSTER_API`-mode clusters via the above; no new endpoints were needed since the
      gap was in the workflow, not the API surface.
- [ ] **Remediation** (CAPI's own MachineHealthCheck-driven replacement) is observed
      (via `GetClusterStatus`'s Machine-level readiness) but not orchestrated — there is
      no platform-initiated "replace this unhealthy machine" action yet.
- [ ] **Provider-specific infrastructure CRs** (the `infrastructureRef` a real
      deployment points at) are deliberately not rendered: no infrastructure provider
      in this codebase is CAPI-aware yet (Phase 6 is still mock-only), so a fabricated
      infra CRD shape would be more misleading than useful.

## Phase 6 — Infrastructure Providers

- [x] Bare Metal provider — the inventory-based core shipped in Phase 1; Phase 6 adds
      the two real, protocol-specific `PowerController`s it was designed around
      (`internal/integrations/baremetal/ipmi.go`, `redfish.go`): IPMI shells out to
      `ipmitool` (no mature pure-Go IPMI stack worth vendoring), with the BMC password
      passed via the `IPMI_PASSWORD` environment variable and `-E`, never as a plain
      CLI argument; Redfish is a hand-rolled REST client against the standard DMTF
      Redfish `ComputerSystem.Reset` action. `Provider` now dispatches to the
      controller registered for each machine's own declared BMC protocol (a fix: it
      previously held one controller for the whole inventory and had no way to know
      which protocol a given machine actually used). Enabled via
      `PLATFORM_BAREMETAL_IPMI_ENABLED`/`PLATFORM_BAREMETAL_REDFISH_ENABLED`; both can
      be on at once for a mixed-protocol inventory.
- [x] Proxmox provider (`internal/integrations/proxmox/client.go`) — a hand-rolled REST
      client against the Proxmox API (API-token authenticated): discovery, cloning the
      configured Talos VM template (`ProvisionMachine`), resource sizing, start/stop/
      reset, deletion, and status. Selected via `PLATFORM_PROXMOX_ADAPTER=real` +
      `PLATFORM_PROXMOX_API_URL`/`_NODE`/`_API_TOKEN`/`_TEMPLATE_VMID`.
- [x] Closed a real gap found while wiring this up: `ports.InfrastructureProvider`
      implementations existed since Phase 1 but nothing ever resolved or called them —
      no registry, no wiring, no reachable API route. Added
      `internal/application/inframanager` (a `infraprovider.Type` -> provider
      registry), a `machine.ProviderMachineID` field (migration `0006` — a machine's
      platform UUID was never the same value the provider uses, e.g. a Proxmox VMID,
      and nothing recorded the latter), and hard power control end-to-end:
      `machineservice.HardPowerOn/Off/Cycle` -> `POST /machines/{id}/power/{on,off,cycle}`
      -> CLI (`talos-platform machine power-{on,off,cycle}`) -> a "Power Cycle" button
      in the UI. This is distinct from the existing `/reboot` (Talos-level, graceful,
      does nothing if the OS is unresponsive) — hard power goes through the BMC/
      hypervisor and works regardless of OS state.
- [x] Extension points documented for AWS/Azure/GCP/Equinix Metal/OpenStack/VMware —
      see [../infrastructure/README.md](../infrastructure/README.md)'s "Adding a new
      provider" checklist.
- [ ] **Known limitation** (same pattern as every other Phase 2-5 adapter): Proxmox and
      the bare-metal power controllers are each one process-wide instance built from
      global configuration, not a distinct instance per organization's own registered
      `infraprovider.InfrastructureProvider` row and credentials.
- [ ] Talos image/template deployment mechanics beyond cloning (building/publishing the
      Talos Proxmox template itself, or the bare-metal PXE/ISO boot pipeline) are
      operator setup, not something this platform automates.

## Phase 7 — Production Hardening

- [x] HA for API/worker/scheduler, distributed workflow locks — `platform-scheduler`
      now uses Redis-lease leader election (`internal/infrastructure/leaderelection`);
      `platform-api`/`platform-worker` were already stateless/lock-free via Postgres
      step claiming (ADR-0005). See [../operations/README.md](operations/README.md#high-availability).
- [x] Vault-backed secrets management — `internal/infrastructure/secrets/vault.go`,
      selected via `PLATFORM_SECRET_STORE_BACKEND=vault` (ADR-0006).
- [x] Full audit coverage of destructive operations — every destructive cluster/machine
      handler now records an audit event including `DENIED` RBAC outcomes
      (`internal/interfaces/http/audit_write.go`).
- [x] Backup/disaster-recovery runbooks — concrete `pg_dump`/`pg_restore` scripts at
      `scripts/backup-postgres.sh` / `scripts/restore-postgres.sh`, documented in
      [../disaster-recovery/README.md](disaster-recovery/README.md).
- [x] Rate limiting, security hardening pass, dependency/secret scanning in CI —
      per-IP token-bucket rate limiting, security headers, CORS allowlist
      (`internal/interfaces/http/middleware`), CI go-version fix, `govulncheck` +
      expanded Trivy scanning (`.github/workflows/ci.yml`).
- [x] Full OpenTelemetry coverage (traces across HTTP, Postgres, workflow engine) —
      `otelhttp` on the API server, a hand-rolled `pgx.QueryTracer`
      (`internal/infrastructure/postgres/tracer.go`), and per-step spans in the
      workflow engine (`internal/workflows/engine.go`). Per-integration
      (Talos/GitHub/ArgoCD/ClusterAPI/Proxmox) spans are not yet instrumented — still
      open.
- [ ] OpenTelemetry metrics/logs export (only traces are wired today; Prometheus
      metrics exist separately via `/metrics` but are not yet correlated with traces).
- [ ] Per-integration client call spans (Talos/GitHub/ArgoCD/ClusterAPI/Proxmox).
- [ ] Automated (scheduled) database backups — the scripts exist but nothing invokes
      them on a schedule yet; that's a deployment-level concern (cron/CronJob) left to
      the operator for now.

## Non-goals (always)

- Replacing Argo CD's reconciliation loop
- Replacing Cluster API's reconciliation loop
- Becoming a second source of truth for desired cluster state (Git is authoritative)
- A Kubernetes controller for every resource — only where continuous in-cluster
  reconciliation is genuinely beneficial
