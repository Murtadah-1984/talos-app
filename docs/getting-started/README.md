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
