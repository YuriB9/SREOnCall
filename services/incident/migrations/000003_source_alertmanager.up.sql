-- Canonicalize alert source: ingestion now writes 'alertmanager' instead of
-- 'prometheus'. Backfill historical rows so grouping rules keyed on
-- 'alertmanager' match existing alerts. Idempotent.
UPDATE incident.incident_alerts SET source = 'alertmanager' WHERE source = 'prometheus';
