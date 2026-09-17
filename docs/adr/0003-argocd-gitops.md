# ADR-0003: Argo CD is the GitOps Reconciliation Engine

## Status
Accepted

## Context
The platform needs to deploy infrastructure components (CNI, CSI, ingress,
cert-manager, observability) and applications onto managed clusters. It would be
possible for the platform to `kubectl apply` these directly, but that makes the
platform a second, competing source of truth and reconciliation loop, defeating the
GitOps model the whole system is built around (see §4 of the product spec).

## Decision
Argo CD is the only component that applies desired Kubernetes state to managed
clusters in steady state. The platform:

1. Generates desired configuration (Helm values, Application/ApplicationSet manifests,
   cluster manifests) as code.
2. Commits it to Git (see [ADR-0004](0004-git-source-of-truth.md)).
3. Ensures the corresponding Argo CD Application/ApplicationSet exists (itself defined
   declaratively in Git, App-of-Apps style) so Argo CD picks up the change.
4. Reads Argo CD's Application status (sync state, health, resource tree, history) via
   the Argo CD API (`internal/integrations/argocd`) for display and drift detection.
5. May request a sync (`POST` to Argo CD's sync endpoint) when an operator explicitly
   asks for it, but does not poll-and-force-sync as its default behavior.
6. Performs rollback by reverting the Git commit, not by mutating cluster state
   directly — Argo CD then reconciles back to the reverted desired state.

`kubectl apply` from the platform is reserved for narrow, explicitly-imperative
bootstrap actions (e.g., installing the initial Argo CD instance itself on a brand new
cluster) — never for ongoing application/infrastructure state.

## Consequences
- The platform needs an `ArgoCDClient` port (`GetApplication`, `ListApplications`,
  `Sync`, `GetHistory`, `Rollback`) implemented against Argo CD's REST/gRPC API.
- Drift is *detected*, not *corrected*, by the platform — correction is Argo CD's job
  (self-heal, if enabled) or an operator's explicit sync/PR action.
- The platform's write path is almost always "commit to Git", not "call Kubernetes API
  to mutate application/infrastructure resources".
