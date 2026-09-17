# ADR-0005: Durable, Resumable Workflow Engine for Long-Running Operations

## Status
Accepted

## Context
Cluster provisioning, upgrades, and machine operations are multi-step, long-running,
and must survive process restarts, partial failures, and retries (§23, §35). A naive
in-memory orchestration (e.g., a goroutine per operation) loses all progress on crash
and cannot be idempotent across duplicate API calls.

## Decision
Introduce a workflow engine (`internal/workflows`) with these properties:

- A **workflow** is a named, versioned sequence of **steps**. Each step is a small,
  idempotent unit of work (e.g., "generate Talos configuration", "create Git branch",
  "wait for Argo CD sync").
- Workflow and step state is persisted in PostgreSQL (`workflows`, `workflow_steps`
  tables) *before* a step executes and updated after it completes/fails, so a crashed
  process can resume from the last completed step.
- Steps are executed by `platform-worker` processes that consume work from RabbitMQ.
  RabbitMQ gives durable delivery, retry with backoff, and horizontal scaling of
  workers.
- Every workflow run is created with a caller-supplied **idempotency key**; starting a
  workflow with a key that already has a non-terminal or successful run returns the
  existing run instead of creating a duplicate (§34).
- Steps that call external systems (Talos, GitHub, Argo CD, infrastructure providers)
  do so through the integration ports (ADR-0001, ADR-0007) with per-step timeout/retry
  policy; failures move the workflow to a `FAILED` or `NEEDS_ATTENTION` state rather
  than silently retrying forever or auto-rolling-back destructively (§35).
- Every step transition emits an event (for the WebSocket/SSE live-update feed, §28) and
  is captured in the audit log where it corresponds to a user-initiated action.

## Consequences
- Business logic for "what happens when I provision a cluster" lives in one
  declarative step list per workflow type, independent of HTTP handlers.
- Horizontal scaling of `platform-worker` is safe because step claims use a
  `SELECT ... FOR UPDATE SKIP LOCKED`-style claim (or a Redis-backed distributed lock
  for the claim window) so two workers never execute the same step concurrently.
- Resuming after an application restart requires no special-case code: the worker just
  picks up any workflow left in a non-terminal state with pending steps.
