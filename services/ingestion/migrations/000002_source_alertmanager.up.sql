-- Canonicalize alert source: ingestion now writes 'alertmanager' instead of
-- 'prometheus'. Backfill historical rows so the source dictionary is consistent.
-- Idempotent: re-running matches no rows once applied.
UPDATE ingestion.raw_alerts SET source = 'alertmanager' WHERE source = 'prometheus';
