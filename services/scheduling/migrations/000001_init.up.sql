CREATE SCHEMA IF NOT EXISTS scheduling;

CREATE TABLE IF NOT EXISTS scheduling.tenants (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS scheduling.users (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    sub                TEXT        NOT NULL UNIQUE,
    preferred_username TEXT        NOT NULL DEFAULT '',
    name               TEXT        NOT NULL DEFAULT '',
    email              TEXT        NOT NULL DEFAULT '',
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS scheduling.tenant_webhook_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  TEXT        NOT NULL,
    token_hash TEXT        NOT NULL UNIQUE,
    source     TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON scheduling.tenant_webhook_tokens (tenant_id);

CREATE TABLE IF NOT EXISTS scheduling.tenant_notification_config (
    tenant_id             TEXT PRIMARY KEY,
    mattermost_webhook_url TEXT NOT NULL DEFAULT '',
    mattermost_channel     TEXT NOT NULL DEFAULT '',
    smtp_from              TEXT NOT NULL DEFAULT '',
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS scheduling.schedules (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      TEXT        NOT NULL,
    name           TEXT        NOT NULL,
    timezone       TEXT        NOT NULL DEFAULT 'UTC',
    rotation       TEXT[]      NOT NULL DEFAULT '{}',
    shift_duration TEXT        NOT NULL,
    start_date     DATE        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON scheduling.schedules (tenant_id);

CREATE TABLE IF NOT EXISTS scheduling.schedule_overrides (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id UUID        NOT NULL REFERENCES scheduling.schedules(id) ON DELETE CASCADE,
    tenant_id   TEXT        NOT NULL,
    user_id     TEXT        NOT NULL,
    start_at    TIMESTAMPTZ NOT NULL,
    end_at      TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT override_window_valid CHECK (end_at > start_at)
);
CREATE INDEX ON scheduling.schedule_overrides (schedule_id, start_at, end_at);
