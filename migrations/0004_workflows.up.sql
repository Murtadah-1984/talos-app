-- Workflow engine, imperative operations, audit log, events, and alerts
-- (§23, §24, §32, ADR-0005).

CREATE TABLE workflows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    cluster_id      UUID REFERENCES clusters(id) ON DELETE SET NULL,
    machine_id      UUID REFERENCES machines(id) ON DELETE SET NULL,
    requested_by    UUID NOT NULL REFERENCES users(id),
    input           JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL,
    current_step    INTEGER NOT NULL DEFAULT 0,
    error           TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workflow_steps (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id  UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    sequence     INTEGER NOT NULL,
    name         TEXT NOT NULL,
    status       TEXT NOT NULL,
    output       JSONB NOT NULL DEFAULT '{}',
    error        TEXT NOT NULL DEFAULT '',
    attempts     INTEGER NOT NULL DEFAULT 0,
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workflow_id, sequence)
);

CREATE TABLE operations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    kind            TEXT NOT NULL,
    cluster_id      UUID REFERENCES clusters(id) ON DELETE SET NULL,
    machine_id      UUID REFERENCES machines(id) ON DELETE SET NULL,
    requested_by    UUID NOT NULL REFERENCES users(id),
    status          TEXT NOT NULL,
    result          TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id    TEXT NOT NULL DEFAULT '',
    actor_id      UUID NOT NULL REFERENCES users(id),
    actor_email   TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL,
    target_kind   TEXT NOT NULL DEFAULT '',
    target_id     TEXT NOT NULL DEFAULT '',
    before_state  JSONB,
    after_state   JSONB,
    result        TEXT NOT NULL,
    ip_address    TEXT NOT NULL DEFAULT '',
    session_info  TEXT NOT NULL DEFAULT '',
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source      TEXT NOT NULL,
    kind        TEXT NOT NULL,
    severity    TEXT NOT NULL,
    target_kind TEXT NOT NULL DEFAULT '',
    target_id   TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL DEFAULT '',
    data        JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alerts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_kind TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    severity    TEXT NOT NULL,
    status      TEXT NOT NULL,
    title       TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    fired_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (target_kind, target_id, title, status)
);

CREATE INDEX idx_workflows_cluster ON workflows(cluster_id);
CREATE INDEX idx_workflows_status ON workflows(status);
CREATE INDEX idx_workflow_steps_workflow ON workflow_steps(workflow_id);
CREATE INDEX idx_operations_cluster ON operations(cluster_id);
CREATE INDEX idx_operations_machine ON operations(machine_id);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_id);
CREATE INDEX idx_audit_logs_target ON audit_logs(target_kind, target_id);
CREATE INDEX idx_events_target ON events(target_kind, target_id);
CREATE INDEX idx_alerts_target ON alerts(target_kind, target_id);
