-- Clusters, cluster templates, and machines (§9, §13, §24).

CREATE TABLE cluster_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    version         TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    provider_mode   TEXT NOT NULL,
    spec            JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name, version)
);

CREATE TABLE clusters (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment_id      UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    site_id             UUID NOT NULL REFERENCES sites(id),
    template_id         UUID REFERENCES cluster_templates(id),
    name                TEXT NOT NULL,
    endpoint            TEXT NOT NULL DEFAULT '',
    provider_mode       TEXT NOT NULL,
    state               TEXT NOT NULL,
    spec                JSONB NOT NULL,
    git_commit_sha      TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE machines (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id          UUID REFERENCES clusters(id) ON DELETE SET NULL,
    site_id             UUID NOT NULL REFERENCES sites(id),
    provider_id         UUID NOT NULL REFERENCES infrastructure_providers(id),
    hostname            TEXT NOT NULL,
    management_ip       TEXT NOT NULL DEFAULT '',
    role                TEXT NOT NULL,
    phase               TEXT NOT NULL,
    talos_version       TEXT NOT NULL DEFAULT '',
    kubernetes_version  TEXT NOT NULL DEFAULT '',
    cpu                 INTEGER NOT NULL DEFAULT 0,
    memory_bytes        BIGINT NOT NULL DEFAULT 0,
    bmc_protocol        TEXT NOT NULL DEFAULT 'NONE',
    bmc_address         TEXT NOT NULL DEFAULT '',
    bmc_credential_ref  TEXT NOT NULL DEFAULT '',
    labels              JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE machine_interfaces (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_id  UUID NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    mac_address TEXT NOT NULL DEFAULT '',
    dhcp        BOOLEAN NOT NULL DEFAULT true,
    addresses   JSONB NOT NULL DEFAULT '[]',
    vlans       JSONB NOT NULL DEFAULT '[]'
);

CREATE TABLE machine_disks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_id  UUID NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
    device      TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    model       TEXT NOT NULL DEFAULT '',
    is_system   BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_clusters_org ON clusters(organization_id);
CREATE INDEX idx_clusters_project ON clusters(project_id);
CREATE INDEX idx_clusters_environment ON clusters(environment_id);
CREATE INDEX idx_clusters_state ON clusters(state);
CREATE INDEX idx_machines_cluster ON machines(cluster_id);
CREATE INDEX idx_machines_site ON machines(site_id);
CREATE INDEX idx_machine_interfaces_machine ON machine_interfaces(machine_id);
CREATE INDEX idx_machine_disks_machine ON machine_disks(machine_id);
