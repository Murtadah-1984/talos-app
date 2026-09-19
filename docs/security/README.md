# Security

See [ADR-0006](../adr/0006-secrets-management.md) for the secrets architecture and §25
of the product spec for the full requirements list. This page covers what's actually
implemented today vs. still open.

## Implemented

- **RBAC**: resource-scoped roles (`PLATFORM_ADMIN` down to `VIEWER`) walking up the
  tenancy hierarchy — `internal/auth/rbac.go`. Enforced on every mutating cluster/machine
  route via `requireRole`/`requireRoleAudited` in `internal/interfaces/http/cluster_handlers.go`
  and `machine_handlers.go`.
- **Authentication**: pluggable `ports.IdentityProvider`. `PLATFORM_AUTH_MODE=dev` issues
  locally-signed HS256 tokens for local development only — **never set this in
  production**. `PLATFORM_AUTH_MODE=oidc` verifies bearer ID tokens from a real OIDC
  provider (Keycloak, Azure AD/Entra ID, GitHub, etc.) via discovery + JWKS
  (`internal/auth/oidc.go`), configured with `PLATFORM_OIDC_ISSUER` and
  `PLATFORM_OIDC_CLIENT_ID`; this is the production path. The frontend completes the
  login flow itself (authorization code + PKCE) and sends the resulting ID token as a
  bearer credential — the platform never sees a password.
- **Secrets at rest**: `internal/infrastructure/secrets` provides two `ports.SecretStore`
  backends selected via `PLATFORM_SECRET_STORE_BACKEND`:
  - `local` (default): AES-256-GCM, keyed by `PLATFORM_SECRET_STORE_KEY_HEX`. Development
    only.
  - `vault`: HashiCorp Vault KV v2, configured via `PLATFORM_VAULT_ADDR`,
    `PLATFORM_VAULT_TOKEN`, `PLATFORM_VAULT_MOUNT_PATH` (see
    [ADR-0006](../adr/0006-secrets-management.md)). This is the production backend.
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
  who/what/when/target/result for every destructive cluster/machine action (provision,
  upgrade, scale, sync, delete, reboot, power control), including `DENIED` results when
  RBAC rejects the request (`requireRoleAudited` in
  `internal/interfaces/http/audit_write.go`). Exposed at `GET /api/v1/audit`.
- **Rate limiting**: per-client-IP token bucket
  (`internal/interfaces/http/middleware/ratelimit.go`), configured via
  `PLATFORM_RATE_LIMIT_RPS` / `PLATFORM_RATE_LIMIT_BURST`. Set `PLATFORM_RATE_LIMIT_RPS=0`
  to disable (e.g. local development).
- **Security headers & CORS**: baseline defensive headers
  (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`,
  `Cross-Origin-Resource-Policy`) on every response, plus an explicit CORS origin
  allowlist via `PLATFORM_CORS_ORIGINS` (comma-separated; empty means same-origin only)
  — `internal/interfaces/http/middleware/security.go`.
- **CI scanning**: Trivy filesystem scan (vulnerabilities, secrets, misconfiguration) and
  `govulncheck` run on every CI build (`.github/workflows/ci.yml`).

## Open (tracked in the roadmap)

- Real OIDC identity provider implementation.
- Encryption-at-rest for the Postgres database itself (deployment-level, not
  application-level).
- A full SAST pass beyond Trivy/govulncheck.

## Reporting a vulnerability

This is a reference implementation; there is no production deployment or bug bounty
program associated with it at this stage.
