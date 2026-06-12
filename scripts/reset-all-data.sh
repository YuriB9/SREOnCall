#!/usr/bin/env bash
#
# DESTRUCTIVE: wipes ALL application data so you can start from a clean slate.
#
# Truncates every table in the service schemas (escalation, incident, ingestion,
# notification, scheduling), including tenants and users. Schema structure and the
# migration-tracking tables (public.*_schema_migrations) are preserved, so the
# services do NOT re-run migrations on next start — they just see empty tables.
#
# The table list is built dynamically from the catalog, so it stays correct as the
# schema evolves. All tables are truncated in one statement with RESTART IDENTITY
# and CASCADE to handle foreign keys and reset sequences.
#
# Usage:
#   scripts/reset-all-data.sh           # show what will be wiped, then prompt
#   scripts/reset-all-data.sh --yes     # skip the confirmation prompt
#   scripts/reset-all-data.sh --dry-run # only list tables, change nothing
#
# Override connection via env vars:
#   PG_CONTAINER (default sreoncall-postgres-1)
#   PG_USER      (default oncall)
#   PG_DB        (default oncall)

set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-sreoncall-postgres-1}"
PG_USER="${PG_USER:-oncall}"
PG_DB="${PG_DB:-oncall}"

# Schemas whose tables hold application data. public is excluded so the
# *_schema_migrations bookkeeping tables survive.
DATA_SCHEMAS="'escalation','incident','ingestion','notification','scheduling'"

MODE="confirm"
case "${1:-}" in
  --yes|-y)  MODE="yes" ;;
  --dry-run) MODE="dry-run" ;;
  "")        MODE="confirm" ;;
  *)
    echo "Unknown argument: $1" >&2
    echo "Usage: $0 [--yes|--dry-run]" >&2
    exit 2
    ;;
esac

psql() {
  docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" "$@"
}

# Fully-qualified, quoted table list (one per line) and a comma-joined version.
TABLES_LIST=$(psql -tA -c "
SELECT format('%I.%I', schemaname, tablename)
FROM pg_tables
WHERE schemaname IN ($DATA_SCHEMAS)
ORDER BY schemaname, tablename;")

if [[ -z "$TABLES_LIST" ]]; then
  echo "No data tables found in schemas: $DATA_SCHEMAS. Nothing to do."
  exit 0
fi

echo "Target: $PG_CONTAINER / database '$PG_DB'"
echo "The following tables will be EMPTIED (TRUNCATE, RESTART IDENTITY, CASCADE):"
echo "$TABLES_LIST" | sed 's/^/  - /'
echo
echo "Preserved: schema structure and public.*_schema_migrations (migration state)."

if [[ "$MODE" == "dry-run" ]]; then
  echo
  echo "Dry-run only — nothing changed."
  exit 0
fi

if [[ "$MODE" == "confirm" ]]; then
  echo
  read -r -p "This permanently deletes all data above. Type 'yes' to proceed: " reply
  if [[ "$reply" != "yes" ]]; then
    echo "Aborted."
    exit 1
  fi
fi

# Comma-join the table list for a single TRUNCATE statement.
TABLES_CSV=$(echo "$TABLES_LIST" | paste -sd, -)

echo "Truncating ..."
psql -c "TRUNCATE TABLE $TABLES_CSV RESTART IDENTITY CASCADE;"

echo "Done. All application data wiped — you can start from scratch."
echo "Restart the services if they cache any state."
