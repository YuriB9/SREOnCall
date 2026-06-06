CREATE SCHEMA IF NOT EXISTS notification;

CREATE TABLE IF NOT EXISTS notification.user_contacts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             TEXT NOT NULL,
    tenant_id           TEXT NOT NULL,
    email               TEXT NOT NULL DEFAULT '',
    mattermost_username TEXT NOT NULL DEFAULT '',
    enabled_channels    TEXT[] NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, tenant_id)
);

CREATE TABLE IF NOT EXISTS notification.notification_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id  TEXT NOT NULL,
    tenant_id    TEXT NOT NULL,
    user_id      TEXT NOT NULL DEFAULT '',
    channel      TEXT NOT NULL,
    status       TEXT NOT NULL,
    recipient    TEXT NOT NULL DEFAULT '',
    error_detail TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON notification.notification_log (incident_id, tenant_id);
CREATE INDEX ON notification.notification_log (tenant_id, created_at);
