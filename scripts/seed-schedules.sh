#!/usr/bin/env bash
# seed-schedules.sh — тестовые данные для UI расписаний дежурств
#
# Использование:
#   bash scripts/seed-schedules.sh [TENANT]
#
# Переменные окружения:
#   PG_DSN   — строка подключения PostgreSQL (по умолчанию: oncall/oncall@localhost:5432)
#   KC_URL   — URL Keycloak (по умолчанию: http://localhost:8090)
#   KC_USER  — admin логин (по умолчанию: admin)
#   KC_PASS  — admin пароль (по умолчанию: admin)
#   KC_REALM — realm с пользователями (по умолчанию: oncall)

set -euo pipefail

TENANT="${1:-team-a}"
PG_DSN="${PG_DSN:-postgres://oncall:oncall@localhost:5432/oncall?sslmode=disable}"
KC_URL="${KC_URL:-http://localhost:8090}"
KC_USER="${KC_USER:-admin}"
KC_PASS="${KC_PASS:-admin}"
KC_REALM="${KC_REALM:-oncall}"

# ── Предварительные проверки ──────────────────────────────────────────────────

for cmd in psql curl python3; do
  command -v "$cmd" &>/dev/null || { echo "Ошибка: $cmd не найден" >&2; exit 1; }
done

psql "$PG_DSN" -c "SELECT 1" &>/dev/null \
  || { echo "Ошибка: нет подключения к PostgreSQL ($PG_DSN)" >&2; exit 1; }

echo "==> Тенант : $TENANT"
echo "==> БД     : $PG_DSN"
echo ""

# ── Keycloak: admin-токен ─────────────────────────────────────────────────────

echo "==> Получаю admin-токен Keycloak ($KC_URL)..."
KC_TOKEN=$(curl -sf --max-time 10 -X POST \
  "$KC_URL/realms/master/protocol/openid-connect/token" \
  --data-urlencode "grant_type=password" \
  --data-urlencode "client_id=admin-cli" \
  --data-urlencode "username=$KC_USER" \
  --data-urlencode "password=$KC_PASS" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")

[ -n "$KC_TOKEN" ] || { echo "Ошибка: не удалось получить токен Keycloak" >&2; exit 1; }

# ── Keycloak: ID группы TENANT ───────────────────────────────────────────────

echo "==> Ищу группу '$TENANT' в realm '$KC_REALM'..."
TENANT_ENC=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$TENANT'))")
GROUP_ID=$(curl -sf --max-time 10 \
  "$KC_URL/admin/realms/$KC_REALM/groups?search=$TENANT_ENC" \
  -H "Authorization: Bearer $KC_TOKEN" \
  | python3 -c "
import json, sys
for g in json.load(sys.stdin):
    if g['name'] == '$TENANT':
        print(g['id']); break
")

[ -n "$GROUP_ID" ] || { echo "Ошибка: группа '$TENANT' не найдена в Keycloak" >&2; exit 1; }
echo "==> Группа: $GROUP_ID"

# ── Keycloak: участники группы ────────────────────────────────────────────────

echo "==> Получаю участников группы..."
MEMBERS_JSON=$(curl -sf --max-time 10 \
  "$KC_URL/admin/realms/$KC_REALM/groups/$GROUP_ID/members?max=200" \
  -H "Authorization: Bearer $KC_TOKEN")

mapfile -t RAW_ROWS < <(echo "$MEMBERS_JSON" | python3 -c "
import json, sys
for u in json.load(sys.stdin):
    un = u.get('username', '')
    if un.startswith('service-account-'):
        continue
    uid = u.get('id', '')
    fn  = u.get('firstName', '')
    ln  = u.get('lastName', '')
    nm  = (fn + ' ' + ln).strip() or un
    em  = u.get('email', '')
    print(f'{uid}|{un}|{nm}|{em}')
")

declare -a UIDS=()
declare -a UNAMES=()
declare -a FULLNAMES=()
declare -a EMAILS=()

for row in "${RAW_ROWS[@]:-}"; do
  [ -z "$row" ] && continue
  IFS='|' read -r _uid _un _nm _em <<< "$row"
  UIDS+=("$_uid")
  UNAMES+=("$_un")
  FULLNAMES+=("$_nm")
  EMAILS+=("$_em")
done

COUNT="${#UIDS[@]}"
echo "==> Участников: $COUNT — ${UNAMES[*]:-}"
echo ""

[ "$COUNT" -ge 2 ] || { echo "Ошибка: в группе '$TENANT' менее 2 пользователей (найдено: $COUNT)" >&2; exit 1; }

# ── Формируем SQL-фрагменты ───────────────────────────────────────────────────

# ARRAY['uuid1', 'uuid2', ...] для ротации
sql_array() {
  local result="ARRAY["
  local sep=""
  for id in "$@"; do
    result+="${sep}'${id}'"
    sep=", "
  done
  result+="]"
  echo "$result"
}

# VALUES для INSERT INTO scheduling.users
USER_VALUES=""
for i in "${!UIDS[@]}"; do
  [ "$i" -gt 0 ] && USER_VALUES+=","$'\n'
  USER_VALUES+="  ('${UIDS[$i]}', '${UNAMES[$i]}', '${FULLNAMES[$i]}', '${EMAILS[$i]}')"
done

# IN (...) для WHERE
IN_CLAUSE=""
local_sep=""
for uid in "${UIDS[@]}"; do
  IN_CLAUSE+="${local_sep}'${uid}'"
  local_sep=", "
done

# Ротации
ROT_ALL=$(sql_array "${UIDS[@]}")
ROT_FIRST2=$(sql_array "${UIDS[0]}" "${UIDS[1]}")
if [ "$COUNT" -ge 3 ]; then
  i0=$((COUNT-3)); i1=$((COUNT-2)); i2=$((COUNT-1))
  ROT_LAST3=$(sql_array "${UIDS[$i0]}" "${UIDS[$i1]}" "${UIDS[$i2]}")
else
  ROT_LAST3=$(sql_array "${UIDS[0]}" "${UIDS[1]}")
fi

# UUID участников для оверрайдов (третий — последний доступный)
OVR_U0="${UIDS[0]}"
OVR_U1="${UIDS[1]}"
OVR_U2="${UIDS[$((COUNT-1))]}"

# ── PostgreSQL: очистка и заполнение ─────────────────────────────────────────

echo "==> Очищаю и заполняю БД..."

psql "$PG_DSN" <<SQL

-- Очистка (schedule_overrides каскадно удаляются вместе с расписаниями)
DELETE FROM scheduling.schedules WHERE tenant_id = '$TENANT';
DELETE FROM scheduling.users     WHERE sub IN ($IN_CLAUSE);
DELETE FROM scheduling.tenants   WHERE slug = '$TENANT';

-- Тенант
INSERT INTO scheduling.tenants (slug, name)
VALUES ('$TENANT', '$TENANT Team');

-- Пользователи из Keycloak
INSERT INTO scheduling.users (sub, preferred_username, name, email)
VALUES
$USER_VALUES
ON CONFLICT (sub) DO UPDATE
  SET preferred_username = EXCLUDED.preferred_username,
      name               = EXCLUDED.name,
      email              = EXCLUDED.email;

-- Расписания и оверрайды
DO \$seed\$
DECLARE
  s1_id UUID;
  s2_id UUID;
  s3_id UUID;
BEGIN

  -- 1. Primary On-Call — еженедельная ротация всех участников тенанта.
  --    start = сегодня - 8 дней → текущий on-call = slot 1 (не первый в списке).
  INSERT INTO scheduling.schedules
    (tenant_id, name, timezone, rotation, shift_duration, start_date)
  VALUES (
    '$TENANT', 'Primary On-Call', 'Europe/Moscow',
    $ROT_ALL, 'P7D',
    (now() - interval '8 days')::date
  ) RETURNING id INTO s1_id;

  -- 2. Database On-Call — ежедневная ротация первых двух участников.
  INSERT INTO scheduling.schedules
    (tenant_id, name, timezone, rotation, shift_duration, start_date)
  VALUES (
    '$TENANT', 'Database On-Call', 'UTC',
    $ROT_FIRST2, 'P1D',
    (now() - interval '3 days')::date
  ) RETURNING id INTO s2_id;

  -- 3. Infra On-Call — двухнедельная ротация последних участников.
  INSERT INTO scheduling.schedules
    (tenant_id, name, timezone, rotation, shift_duration, start_date)
  VALUES (
    '$TENANT', 'Infra On-Call', 'UTC',
    $ROT_LAST3, 'P14D',
    (date_trunc('month', now()) - interval '28 days')::date
  ) RETURNING id INTO s3_id;

  -- Оверрайды равномерно распределены по следующему месяцу (до +30 дней).

  -- Оверрайд 1: ~1-я неделя — первый участник подменяет по Primary.
  INSERT INTO scheduling.schedule_overrides
    (schedule_id, tenant_id, user_id, start_at, end_at)
  VALUES (
    s1_id, '$TENANT', '$OVR_U0',
    now() + interval '5 days',
    now() + interval '7 days'
  );

  -- Оверрайд 2: ~2-я неделя — второй участник подменяет по Database.
  INSERT INTO scheduling.schedule_overrides
    (schedule_id, tenant_id, user_id, start_at, end_at)
  VALUES (
    s2_id, '$TENANT', '$OVR_U1',
    now() + interval '14 days',
    now() + interval '16 days'
  );

  -- Оверрайд 3: ~3-я неделя — третий (или последний) участник подменяет по Infra.
  INSERT INTO scheduling.schedule_overrides
    (schedule_id, tenant_id, user_id, start_at, end_at)
  VALUES (
    s3_id, '$TENANT', '$OVR_U2',
    now() + interval '22 days',
    now() + interval '25 days'
  );

END \$seed\$;

SQL

# ── Итоговая сводка ───────────────────────────────────────────────────────────

echo ""
echo "==> Готово. Расписания тенанта '$TENANT':"
echo ""
psql "$PG_DSN" <<SQL
SELECT
  s.name                       AS расписание,
  s.shift_duration             AS смена,
  array_length(s.rotation, 1)  AS участников,
  s.timezone                   AS tz,
  s.start_date::text           AS старт,
  COUNT(o.id)::int             AS оверрайдов
FROM scheduling.schedules s
LEFT JOIN scheduling.schedule_overrides o ON o.schedule_id = s.id
WHERE s.tenant_id = '$TENANT'
GROUP BY s.id
ORDER BY s.created_at;
SQL

echo ""
echo "Пользователи:"
echo ""
psql "$PG_DSN" <<SQL
SELECT preferred_username AS логин, name AS имя, sub AS uuid
FROM scheduling.users
WHERE sub IN ($IN_CLAUSE)
ORDER BY preferred_username;
SQL
