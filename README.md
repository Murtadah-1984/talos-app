# Talos Platform

A Kubernetes cluster lifecycle and platform management system for clusters running on
[Talos Linux](https://www.talos.dev/) — GitOps-first, Kubernetes-native, and built to
orchestrate/visualize rather than compete with Argo CD, Cluster API, or Kubernetes
controllers. See [docs/architecture.md](docs/architecture.md) for the full design and
[docs/roadmap.md](docs/roadmap.md) for what's implemented today vs. planned.

## What's here (Phase 1 — Foundation)

- **Backend** (`Go`): Clean/Hexagonal architecture — `internal/domain`,
  `internal/application`, `internal/infrastructure`, `internal/interfaces`,
  `internal/integrations`, `internal/workflows`.
- **REST API** (`platform-api`): every resource in the product spec's §27, behind
  authentication + resource-scoped RBAC.
- **Durable workflow engine** (`platform-worker`): cluster provisioning/upgrade/scaling
  workflows, resumable across restarts, dispatched over RabbitMQ.
- **Reconciliation visibility loop** (`platform-scheduler`): a read-only pass that keeps
  health metrics fresh — it never reconciles anything itself.
- **PostgreSQL** schema + migrations for every entity in §24.
- **Frontend** (`React` + `TypeScript` + `Vite`): the sections from §29, including a
  guided cluster-creation flow and per-cluster health/nodes/GitOps/operations tabs.
- **CLI** (`talos-platform`): a thin REST client, no second business layer.
- **Mock adapters** for Talos, GitHub, Argo CD, Cluster API, and Proxmox, so the whole
  system runs and is testable without physical infrastructure. Real implementations
  land in later phases — see the roadmap.

## Local development

Prerequisites: Go 1.23+, Node 22+, Docker (for Postgres/Redis/RabbitMQ/OTel/Jaeger).

```bash
docker compose up -d postgres redis rabbitmq otel-collector jaeger

go run ./cmd/platform-api      # http://localhost:8080
go run ./cmd/platform-worker   # in another terminal
go run ./cmd/platform-scheduler

cd web && npm install && npm run dev   # http://localhost:5173
```

Sign in from the web UI with any email/name — `PLATFORM_AUTH_MODE=dev` (the default)
issues a locally-signed token so the platform is usable without a real OIDC provider.

Or run everything, including the frontend and backends, in containers:

```bash
docker compose up --build
```

## Testing

```bash
go build ./... && go vet ./... && golangci-lint run ./... && go test ./... -race -cover

cd web && npm run typecheck && npm run lint && npm run build
```

## CLI

```bash
go run ./cmd/talos-platform cluster list
go run ./cmd/talos-platform cluster plan <cluster-id>
go run ./cmd/talos-platform workflow list
```

## Documentation

- [Architecture](docs/architecture.md)
- [Roadmap](docs/roadmap.md)
- [ADRs](docs/adr/)
