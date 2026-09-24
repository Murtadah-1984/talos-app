# GitOps

See [ADR-0004](../adr/0004-git-source-of-truth.md) and §3–§4 of the product spec for
the repository layout and workflow this section refers to.

Git is the source of truth for desired cluster configuration. The platform's job is:

1. Render desired configuration as code (`internal/application/gitopsrender`) —
   `cluster.yaml`, `infrastructure.yaml`, `control-plane.yaml`, `workers.yaml`, and
   `argocd-application.yaml` under `gitops/clusters/<name>/`, plus CAPI manifests for
   `CLUSTER_API`-mode clusters.
2. Commit it to a branch and open a pull request (`ports.GitProvider`).
3. Track that change as a `gitops.ChangeSet` (`internal/domain/gitops`), whose status
   moves `COMMITTED` -> `PR_CREATED` -> `MERGED` -> `SYNCED` as the workflow
   progresses. `WorkflowID` and `CommitSHA` are stamped at creation time — the former
   lets a GitHub webhook resume the right workflow (below), the latter lets a change
   be rolled back via Git revert (see [../argocd/README.md](../argocd/README.md)).
4. Wait for a human (or another automated system) to actually merge the PR — see
   "Webhook-driven approval" below — then observe Argo CD reconcile it (see
   [../argocd/README.md](../argocd/README.md)).

## Webhook-driven approval (§4)

The workflow never merges its own PR. Its `await-pull-request-merge` step (and the
CLUSTER_API-mode equivalents, `await-capi-upgrade-merge`/`await-capi-scale-merge`)
checks whether the PR has been merged yet; if not, it returns
`workflows.ErrAwaitingApproval`, which moves the workflow to a new
`AWAITING_APPROVAL` status and stops the engine from dispatching it further — no
polling. `POST /api/v1/webhooks/github`, verified via HMAC-SHA256
(`PLATFORM_GITHUB_WEBHOOK_SECRET`), resolves a GitHub "pull request merged" webhook
event to its `ChangeSet` (by PR URL) and calls `Engine.Resume`, which re-dispatches
the same step — this time it sees the merge and proceeds.

`internal/integrations/github.MockProvider` marks every PR "merged" immediately on
creation (there's no human to wait for in local development), so the mock-backed
pipeline runs straight through without ever pausing or needing a webhook configured.

## Real vs. mock

`ports.GitProvider` has two implementations, selected via `PLATFORM_GITHUB_ADAPTER`:

- `internal/integrations/github.MockProvider` (default) — in-memory, so the full
  commit → PR → merge pipeline can be exercised end-to-end without a live GitHub
  account.
- `internal/integrations/github.Client` (`PLATFORM_GITHUB_ADAPTER=real` +
  `PLATFORM_GITHUB_TOKEN`) — the real GitHub client, using `go-github`'s Git Data API
  so a whole manifest set (several files) commits as one atomic commit rather than
  one API call per file.

See the `CLUSTER_PROVISION` workflow in `internal/workflows/cluster_provision.go` for
exactly how the render/commit/PR/merge/sync steps compose, and
`TestClusterProvisionWorkflow_EndToEnd` in that package for a test exercising the
whole pipeline.

## Bootstrapping a new repository

`RenderClusterManifests` assumes the `gitops/` repository and its top-level layout
already exist — it only ever produces one cluster's `clusters/<name>/` subtree.
`POST /api/v1/gitops/repositories/{id}/scaffold` (`ClusterService.ScaffoldRepository`,
`gitopsrender.RenderRepositoryScaffold`) is what creates that top-level layout in the
first place: `README.md`, `infrastructure/README.md`, `applications/README.md`, and a
root `app-of-apps.yaml` Application watching `applications/` (App-of-Apps pattern) —
committed straight to the repository's default branch. Unlike a cluster change, this
skips the PR/approval flow entirely: there's no existing content yet to conflict with
or review, the same reasoning `git init` plus an initial commit needs no review.

Two preconditions, both pre-existing rather than introduced by this: the
`gitops.GitRepository` record (owner/name/defaultBranch) must already exist — there's
still no HTTP endpoint for creating one, only `GET /api/v1/gitops/repositories` to
list them — and the repository needs at least one commit on its default branch
already (e.g. created with a README on GitHub), since `GitProvider.Commit` resolves
the branch's current tip to build its tree from.

It deliberately doesn't set up automatic per-cluster Application discovery (e.g. an
ApplicationSet with a git directory generator over `clusters/*`) — that would make
each cluster's own `clusters/<name>/argocd-application.yaml` (rendered by
`RenderClusterManifests`) redundant, a bigger redesign than "add the missing
top-level scaffolding" calls for.

## What's not built yet

- **GitHub webhook subscription is a manual setup step.** The platform exposes
  `POST /api/v1/webhooks/github`, but nothing in this codebase registers it as a
  webhook on the target repository — an operator adds it via the GitHub repo/org
  settings UI (or API), pointed at the platform's public URL, subscribed to "Pull
  requests" events, with the same secret as `PLATFORM_GITHUB_WEBHOOK_SECRET`.
