#!/usr/bin/env bash
# fix-keycloak-uids.sh — починить «битых» дежурных после переимпорта realm Keycloak.
#
# Проблема:
#   При локальной разработке Keycloak импортирует юзеров из oncall-realm.json
#   без фиксированных id, поэтому при каждом старте у alice/bob/charlie/diana
#   генерируется новый UUID (sub). А в БД scheduling уже лежат СТАРЫЕ sub:
#     - scheduling.users.sub               (sub -> username)
#     - scheduling.schedules.rotation[]    (массив sub в ротации)
#     - scheduling.schedule_overrides.user_id
#   В результате /oncall возвращает user_id, которого нет в Keycloak (escalation
#   назначает инцидент на призрака, notification шлёт уведомления в пустоту),
#   хотя username ещё резолвится из устаревшей строки scheduling.users.
#
# Решение:
#   Стабильный ключ — username. Берём актуальный маппинг username -> новый sub
#   из Keycloak Admin API и перепрошиваем все старые sub на новые во всех трёх
#   местах одной транзакцией.
#
# Запуск (после старта Keycloak, например в конце dev-up.sh с WITH_KEYCLOAK=1):
#   bash scripts/fix-keycloak-uids.sh
#
# Переменные окружения (со значениями по умолчанию для локалки):
#   KEYCLOAK_BASE          http://localhost:8090
#   KEYCLOAK_REALM         oncall
#   KC_ADMIN_USER          admin            (master realm admin)
#   KC_ADMIN_PASSWORD      admin
#   POSTGRES_SERVICE       postgres         (имя сервиса в docker compose)
#   POSTGRES_USER          oncall
#   POSTGRES_DB            oncall
#   DRY_RUN=1              только показать план, ничего не менять
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="$ROOT/docker-compose.yaml"

KEYCLOAK_BASE="${KEYCLOAK_BASE:-http://localhost:8090}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-oncall}"
KC_ADMIN_USER="${KC_ADMIN_USER:-admin}"
KC_ADMIN_PASSWORD="${KC_ADMIN_PASSWORD:-admin}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-oncall}"
POSTGRES_DB="${POSTGRES_DB:-oncall}"
DRY_RUN="${DRY_RUN:-}"

for bin in curl jq docker; do
  command -v "$bin" >/dev/null 2>&1 || { echo "ОШИБКА: требуется '$bin'" >&2; exit 1; }
done

# psql_exec STDIN-SQL — выполнить SQL в контейнере postgres, tab-separated, без шапки.
psql_exec() {
  docker compose -f "$COMPOSE_FILE" exec -T "$POSTGRES_SERVICE" \
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tA -F $'\t'
}

# SQL-экранирование одинарных кавычек для литералов.
sql_lit() { printf "%s" "${1//\'/\'\'}"; }

echo "==> Получаю admin-токен Keycloak ($KEYCLOAK_BASE)..."
TOKEN="$(curl -fsS \
  -d "client_id=admin-cli" \
  -d "username=$KC_ADMIN_USER" \
  -d "password=$KC_ADMIN_PASSWORD" \
  -d "grant_type=password" \
  "$KEYCLOAK_BASE/realms/master/protocol/openid-connect/token" \
  | jq -r '.access_token')"

if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "ОШИБКА: не удалось получить токен Keycloak" >&2
  exit 1
fi

echo "==> Читаю актуальных пользователей realm '$KEYCLOAK_REALM'..."
# username<TAB>newSub для всех пользователей realm.
declare -A KC_SUB
while IFS=$'\t' read -r username newsub; do
  [[ -z "$username" ]] && continue
  KC_SUB["$username"]="$newsub"
done < <(curl -fsS -H "Authorization: Bearer $TOKEN" \
  "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/users?max=1000&briefRepresentation=true" \
  | jq -r '.[] | [.username, .id] | @tsv')

if [[ ${#KC_SUB[@]} -eq 0 ]]; then
  echo "ОШИБКА: Keycloak вернул пустой список пользователей" >&2
  exit 1
fi
echo "    найдено в Keycloak: ${#KC_SUB[@]} пользователей"

echo "==> Читаю scheduling.users из БД..."
# В scheduling.users за время множества переимпортов накопилось несколько строк
# на один username (каждый UpsertUser по новому sub добавлял строку). Поэтому
# несколько СТАРЫХ sub схлопываются в один НОВЫЙ. Для users это означает не
# построчный UPDATE (упрётся в UNIQUE по sub), а: удалить старые строки и
# оставить одну на username с актуальным sub.
declare -a REMAP_PAIRS=()        # "oldSub newSub" — для rotation[] и overrides
declare -a OLD_SUBS=()           # все старые sub, которые надо убрать из users
declare -A AFFECTED=()           # username -> newSub (одна запись на пользователя)
mapfile -t DB_ROWS < <(printf "SELECT sub, preferred_username FROM scheduling.users;" | psql_exec)

remap_count=0
missing=()
for row in "${DB_ROWS[@]}"; do
  [[ -z "$row" ]] && continue
  oldsub="${row%%$'\t'*}"
  username="${row#*$'\t'}"
  newsub="${KC_SUB[$username]:-}"
  if [[ -z "$newsub" ]]; then
    missing+=("$username (sub=$oldsub)")
    continue
  fi
  if [[ "$oldsub" == "$newsub" ]]; then
    continue   # уже актуально
  fi
  REMAP_PAIRS+=("$oldsub $newsub")
  OLD_SUBS+=("$oldsub")
  AFFECTED["$username"]="$newsub"
  printf "    %-12s %s -> %s\n" "$username" "$oldsub" "$newsub"
  remap_count=$((remap_count + 1))
done

if [[ ${#missing[@]} -gt 0 ]]; then
  echo "ВНИМАНИЕ: эти пользователи из БД не найдены в Keycloak по username:" >&2
  printf "    - %s\n" "${missing[@]}" >&2
fi

if [[ "$remap_count" -eq 0 ]]; then
  echo "==> Нечего чинить: все sub в БД совпадают с Keycloak. ✔"
  exit 0
fi

# ── Сборка транзакции ─────────────────────────────────────────────────────────
# 1) Ссылки old->new в rotation[] и overrides — точечная замена по каждому sub.
# 2) users: удаляем все старые строки и вставляем одну на username с новым sub
#    (схлопывание дублей; ON CONFLICT — если строка с новым sub уже есть).
SQL="BEGIN;"
for pair in "${REMAP_PAIRS[@]}"; do
  old="${pair%% *}"; new="${pair##* }"
  o="$(sql_lit "$old")"; n="$(sql_lit "$new")"
  SQL+="
UPDATE scheduling.schedules          SET rotation=array_replace(rotation,'$o','$n'), updated_at=now() WHERE '$o' = ANY(rotation);
UPDATE scheduling.schedule_overrides SET user_id='$n' WHERE user_id='$o';"
done

# Удаляем все устаревшие строки users одним IN-списком.
del_list=""
for old in "${OLD_SUBS[@]}"; do
  del_list+="${del_list:+,}'$(sql_lit "$old")'"
done
SQL+="
DELETE FROM scheduling.users WHERE sub IN ($del_list);"

# Одна актуальная строка на каждого затронутого пользователя.
for username in "${!AFFECTED[@]}"; do
  n="$(sql_lit "${AFFECTED[$username]}")"; u="$(sql_lit "$username")"
  SQL+="
INSERT INTO scheduling.users (sub, preferred_username, last_seen_at) VALUES ('$n','$u',now())
  ON CONFLICT (sub) DO UPDATE SET preferred_username=EXCLUDED.preferred_username, last_seen_at=now();"
done
SQL+="
COMMIT;"

if [[ -n "$DRY_RUN" ]]; then
  echo "==> DRY_RUN: транзакция не выполнена. Сгенерированный SQL:"
  printf "%s\n" "$SQL"
  exit 0
fi

echo "==> Применяю перепрошивку sub ($remap_count пользователей)..."
printf "%s" "$SQL" | psql_exec >/dev/null

echo "==> Готово. Битые дежурные починены: старые sub заменены на актуальные из Keycloak. ✔"
