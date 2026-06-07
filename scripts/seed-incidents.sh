#!/usr/bin/env bash
# seed-incidents.sh — тестовые данные для дашборда инцидентов
#
# Использование:
#   bash scripts/seed-incidents.sh [TENANT_SLUG]
#
#   TENANT_SLUG — слаг тенанта (по умолчанию: team-a)
#   PG_DSN      — строка подключения (по умолчанию: oncall/oncall@localhost:5432/oncall)
#
# Примеры:
#   bash scripts/seed-incidents.sh
#   bash scripts/seed-incidents.sh team-beta
#   PG_DSN="postgres://user:pass@host:5432/db" bash scripts/seed-incidents.sh

set -euo pipefail

TENANT="${1:-team-a}"
PG_DSN="${PG_DSN:-postgres://oncall:oncall@localhost:5432/oncall?sslmode=disable}"

if ! command -v psql &>/dev/null; then
  echo "Ошибка: psql не установлен." >&2
  exit 1
fi

if ! psql "$PG_DSN" -c "SELECT 1" &>/dev/null; then
  echo "Ошибка: не удалось подключиться к PostgreSQL ($PG_DSN)" >&2
  exit 1
fi

echo "==> Тенант: $TENANT"
echo "==> Очищаю предыдущие данные..."

psql "$PG_DSN" -v "t=$TENANT" <<'SQL'
-- ── Очистка ────────────────────────────────────────────────────────────────

DELETE FROM incident.incident_alerts  WHERE tenant_id = :'t';
DELETE FROM incident.incident_labels  WHERE incident_id IN (SELECT id FROM incident.incidents WHERE tenant_id = :'t');
DELETE FROM incident.incident_comments WHERE tenant_id = :'t';
DELETE FROM incident.incident_history  WHERE tenant_id = :'t';
DELETE FROM incident.incidents         WHERE tenant_id = :'t';

-- ── Инциденты ─────────────────────────────────────────────────────────────

INSERT INTO incident.incidents
  (tenant_id, tenant_slug, title, severity, status, created_at, updated_at)
VALUES
  -- open / critical
  (:'t', :'t', 'PostgreSQL replication lag > 60s',               'critical', 'open', now() - interval '5 minutes',   now()),
  (:'t', :'t', 'Kubernetes pod CrashLoopBackOff: payment-worker','critical', 'open', now() - interval '8 minutes',   now()),

  -- open / high
  (:'t', :'t', 'API p99 latency > 2s on /api/v1/users',          'high',     'open', now() - interval '12 minutes',  now()),
  (:'t', :'t', 'S3 bucket upload failures > 5%',                  'high',     'open', now() - interval '45 minutes',  now()),

  -- open / medium
  (:'t', :'t', 'Certificate expires in 7 days: api.example.com',  'medium',   'open', now() - interval '3 hours',     now()),

  -- open / low
  (:'t', :'t', 'Cron job healthcheck missed: backup-daily',       'low',      'open', now() - interval '6 hours',     now())
;

INSERT INTO incident.incidents
  (tenant_id, tenant_slug, title, severity, status, acknowledged_at, acknowledged_by, created_at, updated_at)
VALUES
  -- acknowledged / critical
  (:'t', :'t', 'Memory usage > 90% on worker-03', 'critical', 'acknowledged',
   now() - interval '20 minutes', 'alice',
   now() - interval '30 minutes', now()),

  -- acknowledged / medium
  (:'t', :'t', 'Disk inode exhaustion on /var/log', 'medium', 'acknowledged',
   now() - interval '50 minutes', 'bob',
   now() - interval '1 hour', now())
;

INSERT INTO incident.incidents
  (tenant_id, tenant_slug, title, severity, status, acknowledged_at, acknowledged_by, resolved_at, created_at, updated_at)
VALUES
  -- resolved / high
  (:'t', :'t', 'Redis eviction rate spike', 'high', 'resolved',
   now() - interval '90 minutes', 'alice',
   now() - interval '30 minutes',
   now() - interval '2 hours', now() - interval '30 minutes'),

  -- resolved / medium
  (:'t', :'t', 'Prometheus scrape target down: node-exporter', 'medium', 'resolved',
   now() - interval '2 hours', 'charlie',
   now() - interval '1 hour',
   now() - interval '4 hours', now() - interval '1 hour')
;

-- ── Лейблы (отображаются как метаданные инцидента) ─────────────────────────

INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'alertname', 'PostgreSQLReplicationLag' FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'PostgreSQL%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'job',       'postgres'                 FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'PostgreSQL%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'instance',  'db-primary-01:9187'       FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'PostgreSQL%';

INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'alertname', 'KubePodCrashLooping'      FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Kubernetes%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'namespace', 'payments'                 FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Kubernetes%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'pod',       'payment-worker-7d9f8b'    FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Kubernetes%';

INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'alertname', 'HighAPILatency'           FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'API p99%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'service',   'user-api'                 FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'API p99%';

INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'alertname', 'S3UploadFailureRate'      FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'S3%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'bucket',    'prod-media-uploads'       FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'S3%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'region',    'eu-west-1'                FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'S3%';

INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'alertname', 'NodeMemoryHigh'           FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Memory%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'instance',  'worker-03:9100'           FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Memory%';

INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'alertname', 'DiskInodeExhaustion'      FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Disk%';
INSERT INTO incident.incident_labels (incident_id, key, value)
SELECT id, 'mountpoint','/var/log'                 FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Disk%';

-- ── Алерты (firing/resolved для вкладки «Алерты» в деталях) ───────────────

INSERT INTO incident.incident_alerts (incident_id, tenant_id, fingerprint, source, status)
SELECT id, :'t', 'fp-pg-replication-lag-001', 'alertmanager', 'firing'
FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'PostgreSQL%';

INSERT INTO incident.incident_alerts (incident_id, tenant_id, fingerprint, source, status)
SELECT id, :'t', 'fp-k8s-crashloop-001', 'alertmanager', 'firing'
FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Kubernetes%';

INSERT INTO incident.incident_alerts (incident_id, tenant_id, fingerprint, source, status)
SELECT id, :'t', 'fp-api-latency-001', 'grafana', 'firing'
FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'API p99%';

INSERT INTO incident.incident_alerts (incident_id, tenant_id, fingerprint, source, status)
SELECT id, :'t', 'fp-redis-eviction-001', 'alertmanager', 'resolved'
FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Redis%';

-- ── История (вкладка «История») ────────────────────────────────────────────

-- created events for all
INSERT INTO incident.incident_history (incident_id, tenant_id, kind, author, old_value, new_value, occurred_at)
SELECT id, :'t', 'status_change', 'system', '', 'open', created_at
FROM incident.incidents WHERE tenant_id = :'t';

-- acknowledged events
INSERT INTO incident.incident_history (incident_id, tenant_id, kind, author, old_value, new_value, occurred_at)
SELECT id, :'t', 'status_change', acknowledged_by, 'open', 'acknowledged', acknowledged_at
FROM incident.incidents WHERE tenant_id = :'t' AND status IN ('acknowledged', 'resolved') AND acknowledged_at IS NOT NULL;

-- resolved events
INSERT INTO incident.incident_history (incident_id, tenant_id, kind, author, old_value, new_value, occurred_at)
SELECT id, :'t', 'status_change', COALESCE(acknowledged_by, 'system'), 'acknowledged', 'resolved', resolved_at
FROM incident.incidents WHERE tenant_id = :'t' AND status = 'resolved' AND resolved_at IS NOT NULL;

-- ── Комментарии (вкладка «Комментарии» для одного инцидента) ──────────────

INSERT INTO incident.incident_comments (incident_id, tenant_id, body, author_id, created_at)
SELECT id, :'t', 'Воспроизводится только под нагрузкой — проверяю конфиг ротации логов.', 'bob', now() - interval '45 minutes'
FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Disk%';

INSERT INTO incident.incident_comments (incident_id, tenant_id, body, author_id, created_at)
SELECT id, :'t', 'Патч задеплоен, ротация запущена вручную. Мониторю metricы.', 'alice', now() - interval '30 minutes'
FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Disk%';

INSERT INTO incident.incident_history (incident_id, tenant_id, kind, author, new_value, occurred_at)
SELECT id, :'t', 'comment_added', 'bob', 'Воспроизводится только под нагрузкой', now() - interval '45 minutes'
FROM incident.incidents WHERE tenant_id = :'t' AND title LIKE 'Disk%';

SQL

echo ""
echo "==> Готово. Сводка по тенанту '$TENANT':"
psql "$PG_DSN" -v "t=$TENANT" <<'SQL'
SELECT
  severity,
  status,
  COUNT(*) AS count
FROM incident.incidents
WHERE tenant_id = :'t'
GROUP BY severity, status
ORDER BY
  CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END,
  CASE status   WHEN 'open' THEN 1 WHEN 'acknowledged' THEN 2 ELSE 3 END;

SELECT COUNT(*) AS total FROM incident.incidents WHERE tenant_id = :'t';
SQL
