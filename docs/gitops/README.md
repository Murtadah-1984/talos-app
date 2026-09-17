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
   moves `PR_CREATED` -> `MERGED` -> `SYNCED` as the workflow progresses.
4. Observe Argo CD reconcile it once merged (see [../argocd/README.md](../argocd/README.md)).

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

## What's not built yet

- **Repository scaffolding.** The renderer produces one cluster's
  `clusters/<name>/` subtree; it assumes the `gitops/` repository and its Argo CD
  App-of-Apps root already exist. Bootstrapping a brand-new repository's top-level
  layout (`infrastructure/`, `applications/`, the App-of-Apps root Applications) is
  not yet built.
- **Webhook-driven approval.** The `await-and-merge-pull-request` workflow step merges
  the PR immediately rather than pausing for a real human review + a GitHub
  PR-merged webhook (§4's approval gate). Fine for exercising the pipeline shape
  today; not fine for production changes that require review.
