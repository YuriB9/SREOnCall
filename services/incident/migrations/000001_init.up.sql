CREATE SCHEMA IF NOT EXISTS incident;

CREATE TABLE IF NOT EXISTS incident.incidents (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        TEXT        NOT NULL,
    tenant_slug      TEXT        NOT NULL DEFAULT '',
    title            TEXT        NOT NULL,
    severity         TEXT        NOT NULL DEFAULT 'info',
    status           TEXT        NOT NULL DEFAULT 'open',
    acknowledged_at  TIMESTAMPTZ,
    acknowledged_by  TEXT,
    resolved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON incident.incidents (tenant_id, status);
CREATE INDEX ON incident.incidents (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS incident.incident_alerts (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id  UUID        NOT NULL REFERENCES incident.incidents(id) ON DELETE CASCADE,
    tenant_id    TEXT        NOT NULL,
    fingerprint  TEXT        NOT NULL,
    source       TEXT        NOT NULL,
    group_key    TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'firing',
    attached_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON incident.incident_alerts (incident_id);
CREATE INDEX ON incident.incident_alerts (tenant_id, fingerprint);
CREATE INDEX ON incident.incident_alerts (tenant_id, group_key, status);

CREATE TABLE IF NOT EXISTS incident.incident_labels (
    incident_id  UUID    NOT NULL REFERENCES incident.incidents(id) ON DELETE CASCADE,
    key          TEXT    NOT NULL,
    value        TEXT    NOT NULL,
    PRIMARY KEY (incident_id, key)
);

CREATE TABLE IF NOT EXISTS incident.incident_comments (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id  UUID        NOT NULL REFERENCES incident.incidents(id) ON DELETE CASCADE,
    tenant_id    TEXT        NOT NULL,
    body         TEXT        NOT NULL,
    author_id    TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON incident.incident_comments (incident_id, created_at ASC);

CREATE TABLE IF NOT EXISTS incident.incident_history (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id  UUID        NOT NULL REFERENCES incident.incidents(id) ON DELETE CASCADE,
    tenant_id    TEXT        NOT NULL,
    kind         TEXT        NOT NULL,
    author       TEXT        NOT NULL DEFAULT '',
    old_value    TEXT        NOT NULL DEFAULT '',
    new_value    TEXT        NOT NULL DEFAULT '',
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON incident.incident_history (incident_id, occurred_at ASC);

CREATE TABLE IF NOT EXISTS incident.incident_grouping_rules (
    tenant_id       TEXT    NOT NULL,
    source          TEXT    NOT NULL,
    grouping_labels TEXT[]  NOT NULL,
    PRIMARY KEY (tenant_id, source)
);
