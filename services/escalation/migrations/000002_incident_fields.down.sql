ALTER TABLE escalation.incident_escalation_states
    DROP COLUMN IF EXISTS incident_title,
    DROP COLUMN IF EXISTS incident_severity,
    DROP COLUMN IF EXISTS incident_status;
