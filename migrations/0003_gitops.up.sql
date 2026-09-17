-- GitOps: Git providers, repositories, configurations, change sets, and
-- cached Argo CD Application status (§19, §20, ADR-0003, ADR-0004).

CREATE TABLE git_providers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    type            TEXT NOT NULL,
    credential_ref  UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE git_repositories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    git_provider_id UUID NOT NULL REFERENCES git_providers(id) ON DELETE CASCADE,
    owner           TEXT NOT NULL,
    name            TEXT NOT NULL,
    default_branch  TEXT NOT NULL DEFAULT 'main',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (git_provider_id, owner, name)
);

CREATE TABLE gitops_configurations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id    UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    repository_id UUID NOT NULL REFERENCES git_repositories(id),
    path          TEXT NOT NULL,
    branch        TEXT NOT NULL DEFAULT 'main',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id)
);

CREATE TABLE gitops_changesets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id      UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    workflow_id     UUID,
    description     TEXT NOT NULL DEFAULT '',
    generated_files JSONB NOT NULL DEFAULT '[]',
    pull_request_url TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL,
    result          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE argo_applications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id      UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    namespace       TEXT NOT NULL DEFAULT '',
    project         TEXT NOT NULL DEFAULT 'default',
    sync_status     TEXT NOT NULL DEFAULT 'Unknown',
    health_status   TEXT NOT NULL DEFAULT 'Unknown',
    revision        TEXT NOT NULL DEFAULT '',
    last_synced_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, name)
);

CREATE TABLE cluster_api_resources (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id  UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    namespace   TEXT NOT NULL DEFAULT '',
    phase       TEXT NOT NULL DEFAULT '',
    ready       BOOLEAN NOT NULL DEFAULT false,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, kind, namespace, name)
);

CREATE INDEX idx_gitops_changesets_cluster ON gitops_changesets(cluster_id);
CREATE INDEX idx_argo_applications_cluster ON argo_applications(cluster_id);
CREATE INDEX idx_capi_resources_cluster ON cluster_api_resources(cluster_id);
