# Getting Started

## Prerequisites

- Go 1.23+
- Node 22+
- Docker (for local Postgres/Redis/RabbitMQ/OpenTelemetry Collector/Jaeger)

## Run the platform locally

```bash
docker compose up -d postgres redis rabbitmq otel-collector jaeger

go run ./cmd/platform-api
go run ./cmd/platform-worker
go run ./cmd/platform-scheduler
```

`platform-api` applies pending database migrations on startup — no separate migration
step is required for local development.

## Run the frontend

```bash
cd web
npm install
npm run dev
```

Open http://localhost:5173 and sign in with any email/name. `PLATFORM_AUTH_MODE=dev`
(the default) issues a locally-signed token; see [../security/README.md](../security/README.md)
for why this must never be enabled in production.

## Everything in containers

```bash
docker compose up --build
```

## Configuration

All configuration is environment-driven (§47) — see
[`internal/infrastructure/config/config.go`](../../internal/infrastructure/config/config.go)
for every variable and its default.

## Talos: mock vs. real

By default (`PLATFORM_TALOS_ADAPTER=mock`) the platform talks to an in-memory
deterministic stand-in for Talos, so machine health/reboot/upgrade and cluster
provisioning work without any real infrastructure. To point at a real Talos cluster:

```bash
export PLATFORM_TALOS_ADAPTER=real
export PLATFORM_TALOS_CONFIG_FILE=/path/to/talosconfig   # from `talosctl config`
```

See [../talos/README.md](../talos/README.md) for what the real client does and its
current limitations.

## GitHub: mock vs. real

By default (`PLATFORM_GITHUB_ADAPTER=mock`) cluster provisioning commits/PRs/merges
against an in-memory stand-in for GitHub. To point at a real repository:

```bash
export PLATFORM_GITHUB_ADAPTER=real
export PLATFORM_GITHUB_TOKEN=ghp_...   # a PAT or GitHub App installation token
```

See [../gitops/README.md](../gitops/README.md) for what the real client does and
what's still open (repository scaffolding, webhook-driven approval).
