# GitOps

See [ADR-0004](../adr/0004-git-source-of-truth.md) and §3–§4 of the product spec for
the repository layout and workflow this section refers to.

Git is the source of truth for desired cluster configuration. The platform's job is:

1. Generate desired configuration as code (`ports.GitProvider.Commit`).
2. Commit it to a branch and open a pull request (`ports.GitProvider.CreatePullRequest`).
3. Track that change as a `gitops.ChangeSet` (`internal/domain/gitops`).
4. Observe Argo CD reconcile it once merged (see [../argocd/README.md](../argocd/README.md)).

Today `ports.GitProvider` is backed by `internal/integrations/github.MockProvider`,
so the full commit → PR → (auto-)merge pipeline can be exercised end-to-end without a
live GitHub account — see the `CLUSTER_PROVISION` workflow in
`internal/workflows/cluster_provision.go` for exactly how the steps compose. The real
GitHub client is Phase 3 — see [../roadmap.md](../roadmap.md).
