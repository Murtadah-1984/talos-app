# ADR-0006: Pluggable, Encrypted Secrets Management

## Status
Accepted

## Context
The platform handles extremely sensitive material: Talos PKI/certificates,
kubeconfigs, GitHub tokens, Proxmox/cloud API credentials, and OIDC client secrets.
None of this may be stored in plaintext in Git or the database, logged, or sent to the
browser (§7, §25, §48).

## Decision
Define a `SecretStore` port in `internal/domain`/`internal/application`:

```go
type SecretStore interface {
    Put(ctx context.Context, ref SecretRef, value []byte) error
    Get(ctx context.Context, ref SecretRef) ([]byte, error)
    Delete(ctx context.Context, ref SecretRef) error
}
```

- **At rest in Git**: any secret material that must live in the GitOps repo (e.g., a
  Kubernetes Secret consumed by a workload) is encrypted client-side before commit using
  a pluggable encryption backend — initially SOPS+age, with the interface designed so
  Vault or Sealed Secrets can be swapped in per repository/environment.
- **At rest in the platform**: credentials referenced by `provider_credentials`,
  `git_providers` etc. are stored as opaque encrypted blobs (envelope-encrypted with a
  key from the configured `SecretStore` backend — HashiCorp Vault in production, a local
  encrypted-at-rest dev backend for local development). The database column holds only a
  reference/ciphertext, never a usable plaintext credential.
- **In transit to the frontend**: never. The browser talks only to the Platform API; the
  API resolves secrets server-side immediately before use and never serializes them into
  an HTTP response (§48).
- **In logs**: a structured-logging redaction layer strips known secret field names and
  any value tagged as sensitive before it reaches log sinks (§25).

## Consequences
- Swapping the secrets backend (dev encrypted-at-rest → Vault) is a configuration
  change, not a code change, because all access goes through `SecretStore`.
- Code review and CI can grep for direct secret serialization into DTOs as a guardrail,
  since the boundary is explicit (DTOs never embed a `SecretRef`'s resolved value).
