-- Backfill tenant_slug for escalation states created from incident.created
-- events that carried an empty tenant_slug. In the event pipeline tenant_id is
-- the tenant slug (the webhook token index in Redis stores the slug), so the
-- copy is exact. Fixes active escalations so the monitor can resolve on-call.
UPDATE escalation.incident_escalation_states SET tenant_slug = tenant_id WHERE tenant_slug = '';
