CREATE SCHEMA IF NOT EXISTS ingestion;

CREATE TABLE IF NOT EXISTS ingestion.raw_alerts (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    TEXT        NOT NULL,
    fingerprint  TEXT        NOT NULL,
    source       TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deduplicated BOOLEAN     NOT NULL DEFAULT false
);

CREATE INDEX ON ingestion.raw_alerts (tenant_id, fingerprint);
CREATE INDEX ON ingestion.raw_alerts (tenant_id, received_at DESC);
