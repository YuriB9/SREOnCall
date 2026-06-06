CREATE SCHEMA IF NOT EXISTS escalation;

CREATE TABLE IF NOT EXISTS escalation.policies (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON escalation.policies (tenant_id);

CREATE TABLE IF NOT EXISTS escalation.policy_tiers (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id          UUID NOT NULL REFERENCES escalation.policies(id) ON DELETE CASCADE,
    tier_number        INT  NOT NULL,
    timeout_seconds    INT  NOT NULL,
    notify_schedule_id TEXT,
    UNIQUE (policy_id, tier_number)
);
CREATE INDEX ON escalation.policy_tiers (policy_id);

CREATE TABLE IF NOT EXISTS escalation.incident_escalation_states (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id  TEXT        NOT NULL UNIQUE,
    tenant_id    TEXT        NOT NULL,
    tenant_slug  TEXT        NOT NULL DEFAULT '',
    policy_id    UUID        NOT NULL REFERENCES escalation.policies(id),
    current_tier INT         NOT NULL DEFAULT 1,
    status       TEXT        NOT NULL DEFAULT 'active',
    escalate_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON escalation.incident_escalation_states (status, escalate_at)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS escalation.escalation_history (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id     TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    event_type      TEXT        NOT NULL,
    tier            INT,
    oncall_user_id  TEXT        NOT NULL DEFAULT '',
    oncall_username TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON escalation.escalation_history (incident_id, created_at);

CREATE TABLE IF NOT EXISTS escalation.tenant_escalation_config (
    tenant_id         TEXT PRIMARY KEY,
    default_policy_id UUID REFERENCES escalation.policies(id) ON DELETE SET NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
