# Security

See [ADR-0006](../adr/0006-secrets-management.md) for the secrets architecture and §25
of the product spec for the full requirements list. This page covers what's actually
implemented today vs. still open.

## Implemented (Phase 1)

- **RBAC**: resource-scoped roles (`PLATFORM_ADMIN` down to `VIEWER`) walking up the
  tenancy hierarchy — `internal/auth/rbac.go`. Enforced on every mutating cluster/machine
  route via `requireRole` in `internal/interfaces/http/cluster_handlers.go` and
  `machine_handlers.go`.
- **Authentication**: pluggable `ports.IdentityProvider`. `PLATFORM_AUTH_MODE=dev` issues
  locally-signed HS256 tokens for local development only — **never set this in
  production**. OIDC (Keycloak/Azure AD/GitHub) is the production path and is not yet
  implemented (see [../roadmap.md](../roadmap.md) Phase 2+).
- **Secrets at rest**: `internal/infrastructure/secrets` provides an AES-256-GCM local
  backend for development. Vault integration is the production backend and is not yet
  implemented — the `ports.SecretStore` interface is designed so it drops in without
  touching call sites.
- **No secrets in logs**: structured logging (`slog`) never receives raw credential
  values; nothing in the codebase serializes a `SecretRef`'s resolved value into an API
  response.
- **Talos credentials never reach the browser**: the frontend only ever calls the
  Platform API (§48); Talos endpoints/PKI are resolved server-side inside
  `internal/integrations/talos`.
- **Idempotency**: every workflow/operation-creating endpoint accepts an
  `Idempotency-Key` header (§34), preventing duplicate cluster provisioning or machine
  operations from a retried request.
- **Audit logging**: `internal/domain/audit` + `internal/infrastructure/postgres` record
  who/what/when/target/result for every audited action; exposed at `GET /api/v1/audit`.

## Open (tracked in the roadmap)

- Real OIDC identity provider implementation.
- Vault-backed `SecretStore` implementation.
- Rate limiting.
- Encryption-at-rest for the Postgres database itself (deployment-level, not
  application-level).
- Dependency/secret scanning is wired into CI (Trivy filesystem scan); a full SAST pass
  is not yet part of this phase.

## Reporting a vulnerability

This is a reference implementation; there is no production deployment or bug bounty
program associated with it at this stage.
