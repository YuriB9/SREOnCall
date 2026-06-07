#!/usr/bin/env bash
# seed-escalations.sh — тестовые политики эскалации
#
# Использование:
#   bash scripts/seed-escalations.sh [TENANT]
#
# Переменные окружения:
#   PG_DSN — строка подключения PostgreSQL (по умолчанию: oncall/oncall@localhost:5432)
#
# Требует запущенных миграций: scheduling.schedules и escalation.*

set -euo pipefail

TENANT="${1:-team-a}"
PG_DSN="${PG_DSN:-postgres://oncall:oncall@localhost:5432/oncall?sslmode=disable}"

# ── Предварительные проверки ──────────────────────────────────────────────────

command -v psql &>/dev/null || { echo "Ошибка: psql не найден" >&2; exit 1; }

psql "$PG_DSN" -c "SELECT 1" &>/dev/null \
  || { echo "Ошибка: нет подключения к PostgreSQL ($PG_DSN)" >&2; exit 1; }

echo "==> Тенант : $TENANT"
echo "==> БД     : $PG_DSN"
echo ""

# ── Получаем UUID расписаний тенанта ─────────────────────────────────────────

echo "==> Читаю расписания тенанта '$TENANT' из scheduling.schedules..."

mapfile -t SCHED_ROWS < <(psql "$PG_DSN" -t -A -F'|' <<SQL
SELECT id, name
FROM scheduling.schedules
WHERE tenant_id = '$TENANT'
ORDER BY created_at;
SQL
)

declare -A SCHED_BY_NAME=()
declare -a SCHED_IDS=()
declare -a SCHED_NAMES=()

for row in "${SCHED_ROWS[@]:-}"; do
  [ -z "$row" ] && continue
  IFS='|' read -r _id _name <<< "$row"
  SCHED_BY_NAME["$_name"]="$_id"
  SCHED_IDS+=("$_id")
  SCHED_NAMES+=("$_name")
done

COUNT="${#SCHED_IDS[@]}"
echo "==> Расписаний: $COUNT — ${SCHED_NAMES[*]:-}"
echo ""

[ "$COUNT" -ge 1 ] || {
  echo "Ошибка: нет расписаний для тенанта '$TENANT'." >&2
  echo "        Запустите seed-schedules.sh перед этим скриптом." >&2
  exit 1
}

# Назначаем слоты: используем то, что есть (от 1 до 3 расписаний).
# S1 = Primary On-Call / первое расписание
# S2 = Database On-Call / второе (или снова S1 если одно)
# S3 = Infra On-Call   / третье (или S1 если меньше трёх)
S1="${SCHED_IDS[0]}"
S2="${SCHED_IDS[$((COUNT >= 2 ? 1 : 0))]}"
S3="${SCHED_IDS[$((COUNT >= 3 ? 2 : 0))]}"

N1="${SCHED_NAMES[0]}"
N2="${SCHED_NAMES[$((COUNT >= 2 ? 1 : 0))]}"
N3="${SCHED_NAMES[$((COUNT >= 3 ? 2 : 0))]}"

echo "==> Слоты политик:"
echo "    Tier-1 расписание: $N1"
echo "    Tier-2 расписание: $N2"
echo "    Tier-3 расписание: $N3"
echo ""

# ── PostgreSQL: очистка и заполнение ─────────────────────────────────────────

echo "==> Очищаю и заполняю escalation.*..."

psql "$PG_DSN" <<SQL

-- Очистка (каскад удаляет tiers, states, history)
DELETE FROM escalation.tenant_escalation_config WHERE tenant_id = '$TENANT';
DELETE FROM escalation.policies WHERE tenant_id = '$TENANT';

DO \$seed\$
DECLARE
  p1_id UUID;
  p2_id UUID;
  p3_id UUID;
BEGIN

  -- ── Политика 1: Основная эскалация ───────────────────────────────────────
  -- 2 уровня: дежурный инженер → дежурный по инфраструктуре.
  INSERT INTO escalation.policies (tenant_id, name)
  VALUES ('$TENANT', 'Основная эскалация')
  RETURNING id INTO p1_id;

  INSERT INTO escalation.policy_tiers
    (policy_id, tier_number, timeout_seconds, notify_schedule_id)
  VALUES
    (p1_id, 1,  900, '$S1'),   -- 15 мин → первый дежурный
    (p1_id, 2, 1800, '$S2');   -- 30 мин → второй дежурный

  -- ── Политика 2: Эскалация базы данных ────────────────────────────────────
  -- 2 уровня: дежурный по БД → первичный дежурный.
  INSERT INTO escalation.policies (tenant_id, name)
  VALUES ('$TENANT', 'Эскалация базы данных')
  RETURNING id INTO p2_id;

  INSERT INTO escalation.policy_tiers
    (policy_id, tier_number, timeout_seconds, notify_schedule_id)
  VALUES
    (p2_id, 1,  600, '$S2'),   -- 10 мин → дежурный по БД
    (p2_id, 2, 1800, '$S1');   -- 30 мин → первичный

  -- ── Политика 3: Критическая эскалация ────────────────────────────────────
  -- 3 уровня: быстрая цепочка для P0/P1 инцидентов.
  INSERT INTO escalation.policies (tenant_id, name)
  VALUES ('$TENANT', 'Критическая эскалация')
  RETURNING id INTO p3_id;

  INSERT INTO escalation.policy_tiers
    (policy_id, tier_number, timeout_seconds, notify_schedule_id)
  VALUES
    (p3_id, 1,  300, '$S1'),   --  5 мин → первичный
    (p3_id, 2,  600, '$S2'),   -- 10 мин → вторичный
    (p3_id, 3, 1200, '$S3');   -- 20 мин → третичный

  -- ── Политика по умолчанию ────────────────────────────────────────────────
  INSERT INTO escalation.tenant_escalation_config (tenant_id, default_policy_id)
  VALUES ('$TENANT', p1_id)
  ON CONFLICT (tenant_id) DO UPDATE
    SET default_policy_id = EXCLUDED.default_policy_id,
        updated_at        = now();

END \$seed\$;

SQL

# ── Итоговая сводка ───────────────────────────────────────────────────────────

echo ""
echo "==> Готово. Политики эскалации тенанта '$TENANT':"
echo ""
psql "$PG_DSN" <<SQL
SELECT
  p.name                                                   AS политика,
  COUNT(t.id)::int                                         AS уровней,
  MIN(t.timeout_seconds) || '–' || MAX(t.timeout_seconds)  AS "таймаут (сек)",
  CASE WHEN cfg.default_policy_id = p.id THEN 'да' ELSE '' END AS "по умолч."
FROM escalation.policies p
LEFT JOIN escalation.policy_tiers t ON t.policy_id = p.id
LEFT JOIN escalation.tenant_escalation_config cfg ON cfg.tenant_id = p.tenant_id
WHERE p.tenant_id = '$TENANT'
GROUP BY p.id, p.name, cfg.default_policy_id
ORDER BY p.created_at;
SQL
