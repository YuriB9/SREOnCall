# Release Notes

## Unreleased — Canonicalize alert source `alertmanager`

Ingestion now writes `source=alertmanager` (was `prometheus`) for Alertmanager
webhooks, so administrator-defined grouping rules for `alertmanager` finally apply.
Backfill migrations rewrite historical `source` values in `ingestion.raw_alerts`
and `incident.incident_alerts`.

**Known limitation — deploy window.** An alert's fingerprint is
`SHA-256(labels + source + tenant)`. A firing alert accepted **before** this
deploy carries `source=prometheus` in its fingerprint; its matching `resolved`
notification arrives **after** the deploy with `source=alertmanager` and a
different fingerprint. `ResolveAlert` will not find the original firing alert
(an unmatched resolved is ignored without error, per spec), so the incident
**stays open and must be closed manually**.

The risk window is limited to alerts active at the moment of deploy. Stored
fingerprints cannot be recomputed (`incident_alerts` does not retain labels).
To minimize impact, deploy during a window with few active alerts.
