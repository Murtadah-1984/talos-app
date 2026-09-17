# Argo CD Integration

See [ADR-0003](../adr/0003-argocd-gitops.md).

The platform observes Argo CD Application sync/health status through
`ports.ArgoCDClient` (`internal/application/ports/argocd.go`), backed today by
`internal/integrations/argocd.MockClient`. It requests a sync when a workflow step or
operator explicitly asks for one; it never runs its own reconciliation loop.

A cluster's live Argo CD status is available at `GET /api/v1/clusters/{id}/gitops` and
shown on the cluster detail page's GitOps tab.

The real Argo CD REST/gRPC client is Phase 4 — see [../roadmap.md](../roadmap.md).
