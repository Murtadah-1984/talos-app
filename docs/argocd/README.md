# Argo CD Integration

See [ADR-0003](../adr/0003-argocd-gitops.md).

The platform observes Argo CD Application sync/health status through
`ports.ArgoCDClient` (`internal/application/ports/argocd.go`). Two implementations
exist, selected via `PLATFORM_ARGOCD_ADAPTER`:

- `internal/integrations/argocd.MockClient` (default) — in-memory, seeded by the
  cluster provisioning workflow to simulate Argo CD picking up a merged change.
- `internal/integrations/argocd.Client` (`PLATFORM_ARGOCD_ADAPTER=real` +
  `PLATFORM_ARGOCD_SERVER_URL` + `PLATFORM_ARGOCD_TOKEN`) — a hand-rolled REST client
  against Argo CD's documented HTTP API, not the official Go SDK (which pulls in most
  of k8s client-go for what is, from this platform's side, a read-mostly status/sync
  client).

## Where sync-status data comes from

- **On demand**: `GET /api/v1/clusters/{id}/gitops` reads the cached
  `gitops.ArgoApplication` row(s) and recent change sets, shown on the cluster detail
  page's GitOps tab.
- **Continuously**: `platform-scheduler`'s reconciliation loop (`cmd/platform-scheduler`)
  calls `GetApplication` for every Argo-CD-enabled cluster on each tick, refreshes that
  cache, and fires/resolves `audit.Alert`s for drift (`OutOfSync`) and degraded health —
  a visibility pass only; it never syncs or reconciles anything itself.
- **On request**: `POST /api/v1/clusters/{id}/sync` (`ClusterService.TriggerSync`) calls
  `ArgoCDClient.Sync` directly. This is the one place the cluster service talks to an
  integration port outside a workflow — it's an imperative "sync now" request, not a
  desired-state change, the same reasoning that lets `machineservice` call
  `TalosClient` directly for reboot/upgrade (ADR-0001).

## What's not built yet

- **ApplicationSet discovery** — only individual Applications are read today.
- **Rollback via Git revert** (§18, §20) — needs the commit SHA a change produced
  (not yet persisted on `gitops.ChangeSet`) and a way to resolve/re-commit a prior
  state. See [../roadmap.md](../roadmap.md).
