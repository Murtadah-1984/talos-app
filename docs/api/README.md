# API

The REST API is implemented in `internal/interfaces/http` and follows §27 of the
product spec. A full OpenAPI specification is not yet generated (tracked in
[../roadmap.md](../roadmap.md), Phase 1 follow-up) — until then, the handler
registrations in `internal/interfaces/http/server.go` and each `mount*` function are
the source of truth for available routes.

## Authentication

Every route under `/api/v1` except `POST /api/v1/auth/login` requires
`Authorization: Bearer <token>`. See [../security/README.md](../security/README.md).

## Idempotency

Endpoints that create a workflow or operation (`provision`, `upgrade`, `scale`,
`delete`, machine `reboot`/`upgrade`) accept an `Idempotency-Key` header. Omitting it
falls back to a deterministic per-action key, so accidental retries still can't create
duplicate work (§34).

## Real-time updates

`GET /api/v1/events/stream` is a Server-Sent Events stream (§28) — see
`internal/interfaces/websocket/hub.go`.
