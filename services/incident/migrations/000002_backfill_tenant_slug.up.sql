-- Backfill tenant_slug for incidents created by the alert consumer before it
-- started filling the field. In the event pipeline tenant_id is the tenant
-- slug (the webhook token index in Redis stores the slug), so the copy is exact.
UPDATE incident.incidents SET tenant_slug = tenant_id WHERE tenant_slug = '';
