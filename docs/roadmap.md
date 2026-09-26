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
- [x] Multi-cluster credential handling — closed in Phase 8, see below.
- [x] Machine discovery (enumerating not-yet-known machines) — clarified, not built:
      this is really an infrastructure-provider concern (bare metal/Proxmox
      `DiscoverMachines`, §10, already real since Phase 6) once a machine already has
      an endpoint; nothing further belonged on the Talos side.
- [x] Cluster discovery via Talos (`DIRECT_TALOS` mode) — closed in Phase 8, see below.
- [x] Maintenance-mode (insecure, pre-PKI) connections — closed in Phase 8, see below.

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
- [x] **GitOps repository scaffolding generator** and **webhook-driven human-in-the-loop
      approval gate** (§4) — both closed in Phase 8, see below.

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
- [x] **ApplicationSet discovery** and **rollback via Git revert** (§18, §20) — both
      closed in Phase 8, see below.

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
- [x] **Remediation** (CAPI's own MachineHealthCheck-driven replacement) is now
      directly observed — `GetClusterStatus` lists MachineHealthCheck resources for the
      cluster alongside Machine-level readiness (Phase 8) — but still not orchestrated:
      there is no platform-initiated "replace this unhealthy machine" action, by
      design (ADR-0002 — that's CAPI's own controller's job).
- [x] **Provider-specific infrastructure CRs** — closed in Phase 8 for Proxmox, see
      below; bare metal has no comparably established CAPI infrastructure provider to
      target and remains CAPI-unaware (`DIRECT_TALOS` mode is its path).

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
- [x] OpenTelemetry metrics/logs export, per-integration client call spans, and
      automated scheduled database backups — all closed in Phase 8, see below.

## Phase 8 — Closing the gaps

Every remaining "still open" item carried forward from Phases 1-7, in one place. This
phase has no new subsystem of its own — it's aimed at eliminating every documented
limitation so the mock-vs-real story is complete everywhere.

- [x] Real OIDC identity provider (`PLATFORM_AUTH_MODE=oidc`) — `internal/auth/oidc.go`
      verifies bearer ID tokens via OIDC discovery + JWKS
      (`github.com/coreos/go-oidc/v3`), configured with `PLATFORM_OIDC_ISSUER` /
      `PLATFORM_OIDC_CLIENT_ID`.
- [x] Automated (scheduled) database backups — a Helm `CronJob`
      (`deploy/helm/platform/templates/platform-backup-cronjob.yaml`) runs `pg_dump`
      into a PVC on a schedule (`backup.schedule`), pruning by `backup.retentionDays`.
      Off-cluster replication of that PVC is still an operator responsibility.
- [x] Per-integration OTel client spans (Talos/GitHub/ArgoCD/ClusterAPI/Proxmox) —
      transport-level instrumentation: `otelhttp.NewTransport` on the ArgoCD, Proxmox,
      and GitHub (`go-github`) HTTP clients; `otelgrpc.NewClientHandler` as a gRPC
      stats handler on the Talos client's dial; `rest.Config.WrapTransport` on the
      Cluster API dynamic client. One span per real backend call, with no changes
      needed to each method's business logic.
- [x] OpenTelemetry metrics/logs export — `observability.InitMetrics`
      (`internal/observability/otel.go`) exports OTel metrics to the same OTLP
      collector traces go to; once set, `otelhttp`'s existing instrumentation (already
      wrapping the API server and every real integration client) emits request
      duration/count metrics through it automatically. HTTP request log lines now
      carry `trace_id`/`span_id` when a span is active
      (`internal/interfaces/http/middleware/logging.go`), so a log line pivots
      straight to its trace. Also fixed a real gap found along the way:
      `platform-scheduler` never called `InitTracing` at all — it now does, plus a
      span around each reconciliation tick. Metrics/logs export elsewhere (workflow
      engine, integration clients) still only has traces, not correlated log lines —
      wiring every `slog` call site through `*Context` variants everywhere would be a
      much larger mechanical change, deliberately not done here.
- [x] Argo CD **ApplicationSet discovery** — `ArgoCDClient.ListApplicationSets`
      (`internal/integrations/argocd/client.go`), exposed as a live passthrough (not a
      cached domain entity, unlike individual Applications) at
      `GET /api/v1/clusters/{id}/gitops/applicationsets`.
- [x] Argo CD **rollback via Git revert** (§18, §20) —
      `clusterservice.Service.Rollback` (`internal/application/clusterservice/service.go`),
      exposed as `POST /api/v1/clusters/{id}/changesets/{changeSetId}/rollback`. Required
      persisting the commit SHA on `gitops.ChangeSet` (migration 0007) and a new
      `GitProvider.GetCommitParent` port method. Reverts only the file changes a change
      set made — not side effects a workflow step took outside Git (e.g. a direct Talos
      call in `DIRECT_TALOS` mode).
- [x] **GitOps repository scaffolding generator** — `gitopsrender.RenderRepositoryScaffold`
      renders the top-level `gitops/` layout (README, `infrastructure/`,
      `applications/`, a root App-of-Apps Application), and
      `ClusterService.ScaffoldRepository` commits it straight to the repository's
      default branch (no PR — there's nothing yet to review), exposed as
      `POST /api/v1/gitops/repositories/{id}/scaffold`. The `GitRepository` record
      itself must already exist (`CreateRepository`, still no HTTP endpoint of its
      own — a pre-existing gap, not introduced here) and the repository needs at
      least one existing commit on its default branch (e.g. created with a README),
      since `Commit` resolves the branch's current tip to build from — the same
      precondition every other commit path in this codebase already has.
- [x] **Webhook-driven human-in-the-loop approval gate** (§4) — the
      `await-pull-request-merge`/`await-capi-*-merge` workflow steps no longer merge a
      PR themselves; they check whether it's merged yet and, if not, return
      `workflows.ErrAwaitingApproval`, which the engine turns into a new
      `AWAITING_APPROVAL` workflow status and stops dispatching (no polling — the
      workflow sits idle). `POST /api/v1/webhooks/github`
      (`internal/interfaces/http/webhook_handlers.go`), verified via HMAC-SHA256
      (`PLATFORM_GITHUB_WEBHOOK_SECRET`), resolves a GitHub "pull request merged"
      event to its `gitops.ChangeSet` (now via `GetChangeSetByPullRequestURL`, and
      `ChangeSet.WorkflowID` is now actually populated at creation time) and calls
      `Engine.Resume`, which re-dispatches the same paused step. The mock
      `GitProvider` marks every PR "merged" immediately on creation, so local
      dev/tests still complete without a webhook. `POST /api/v1/workflows/{id}/resume`
      is a manual fallback for repositories that haven't registered the webhook.
- [x] Cluster API **remediation observation** (observing CAPI's own
      MachineHealthCheck-driven replacement, not initiating it) — see Phase 5.
- [x] Cluster API **provider-specific infrastructure CRs** for Proxmox —
      `clusterapi.ProxmoxInfrastructure` (`internal/integrations/clusterapi/client.go`)
      renders `ProxmoxCluster`/`ProxmoxMachineTemplate`
      (`infrastructure.cluster.x-k8s.io/v1alpha1`, from
      [cluster-api-provider-proxmox](https://github.com/ionos-cloud/cluster-api-provider-proxmox)
      "CAPMOX") wired into `Cluster.spec.infrastructureRef`,
      `TalosControlPlane.spec.infrastructureTemplate`, and each worker pool's
      `infrastructureRef`, whenever `PLATFORM_PROXMOX_ADAPTER=real` — reusing the same
      global Proxmox config the real `InfrastructureProvider` client already uses.
      Zero value (Proxmox not configured) renders exactly as before: no
      `infrastructureRef` at all. AWSCluster/vSphereCluster/etc. remain unbuilt — no
      other infrastructure provider in this codebase is CAPI-aware, and bare metal has
      no comparably established CAPI provider to target.
- [x] Talos **cluster discovery** (`TalosClient.DiscoverClusterMembers`, inferring an
      existing cluster's topology in `DIRECT_TALOS` mode from one seed endpoint, via
      Talos's own discovery-service-backed Member resource), exposed as a live,
      read-only `GET /api/v1/machines/discover?endpoint=<ip>`. Deliberately doesn't
      create `machine.Machine` records: doing that needs a
      registration/import path this codebase doesn't have yet (every existing machine
      record today comes from an infrastructure provider's own `ProvisionMachine`
      call, which an externally provisioned cluster's nodes never went through).
      "Machine discovery" (enumerating not-yet-known machines) was already correctly
      scoped as an infrastructure-provider concern, not a Talos one — see Phase 6's
      `DiscoverMachines`.
- [x] Talos **maintenance-mode (insecure, pre-PKI) connections**
      (`TalosClient.ApplyMaintenanceConfiguration`) for freshly-booted, not-yet-joined
      nodes, with optional TLS certificate fingerprint pinning. Not yet called from any
      workflow — no bare-metal/Proxmox provisioning step pushes an initial
      configuration yet (Phase 6 providers create VMs/power on machines; they don't
      drive first-boot Talos config), so this lands as an available capability, the
      same way `ApplyMachineConfiguration` itself has no caller yet either.
- [x] Talos multi-cluster credential handling — `TalosClient.EnsureCredentials`
      registers a talosconfig against a specific machine endpoint;
      `talos.Client.dial` resolves per-endpoint credentials from that registry,
      falling back to the adapter's single default talosconfig when nothing's been
      explicitly registered (so single-cluster deployments, the common case, are
      unaffected). `cluster.Cluster.TalosConfigRef` (migration 0008) points a
      cluster at its own talosconfig in the SecretStore; `machineservice.Service`
      and the `CLUSTER_PROVISION` workflow's `checkTalosHealth` both resolve and
      register it before any Talos call for a machine belonging to that cluster.
      `ports.TalosClient` methods are still keyed only by endpoint, not cluster ID
      — this is endpoint-scoped credential resolution, not a signature change to
      the whole interface.

## Non-goals (always)

- Replacing Argo CD's reconciliation loop
- Replacing Cluster API's reconciliation loop
- Becoming a second source of truth for desired cluster state (Git is authoritative)
- A Kubernetes controller for every resource — only where continuous in-cluster
  reconciliation is genuinely beneficial
