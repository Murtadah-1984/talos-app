# Development Guide

## Repository layout

```
cmd/                      entry points (platform-api, platform-worker, platform-scheduler, CLI)
internal/domain/          entities, value objects — no external dependencies
internal/application/     use cases + ports (interfaces to external systems)
internal/infrastructure/  postgres, redis, rabbitmq, config, secrets — implements ports
internal/integrations/    talos, github, argocd, clusterapi, proxmox, baremetal
internal/interfaces/      REST handlers/middleware, WebSocket/SSE
internal/workflows/       the durable workflow engine + workflow definitions
internal/observability/   OpenTelemetry + Prometheus wiring
internal/auth/            authentication + RBAC
migrations/               SQL migrations (embedded into platform-api/platform-worker)
web/                      React/TypeScript frontend
deploy/helm/              Helm chart for platform-api/worker/scheduler/web
```

See [../architecture.md](../architecture.md) for the reasoning behind this layout and
the [ADRs](../adr/) for specific decisions.

## Adding a new integration

1. Define the port (interface) in `internal/application/ports/` if one doesn't exist.
2. Implement it under `internal/integrations/<name>/` — start with a mock
   implementation (see `internal/integrations/talos/mock.go` for the pattern) so the
   rest of the system can be built and tested against it immediately.
3. Wire the mock into `cmd/platform-api/main.go` / `cmd/platform-worker/main.go` behind
   an adapter-mode config flag (see `PLATFORM_TALOS_ADAPTER` etc. in
   `internal/infrastructure/config/config.go`).
4. Add the real implementation later behind the same port — nothing above it changes.

## Adding a new workflow

1. Add a `workflow.Type` constant in `internal/domain/workflow/workflow.go`.
2. Write a `workflows.Definition` (see `internal/workflows/cluster_provision.go`) as an
   ordered list of small, idempotent `StepDefinition`s.
3. Register it in both `cmd/platform-api` (so `Enqueue` can validate the type) and
   `cmd/platform-worker` (so a worker can execute it).

## Running checks locally

```bash
gofmt -l .            # must be empty
go vet ./...
golangci-lint run ./...
go test ./... -race -cover

cd web
npm run format:check
npm run lint
npm run typecheck
npm run build
```

## Database migrations

Add a new pair of files to `migrations/`: `NNNN_description.up.sql` and
`NNNN_description.down.sql`. They're embedded at build time
(`migrations/embed.go`) and applied automatically on `platform-api`/`platform-worker`
startup via `internal/infrastructure/postgres/migrate.go`.
