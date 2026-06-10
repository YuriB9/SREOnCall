-- Incident data carried into escalation.triggered events (enrich-notifications).
-- Nullable: existing rows predate the feature and are read back as ''.
ALTER TABLE escalation.incident_escalation_states
    ADD COLUMN IF NOT EXISTS incident_title    TEXT,
    ADD COLUMN IF NOT EXISTS incident_severity TEXT,
    ADD COLUMN IF NOT EXISTS incident_status   TEXT;
