#!/usr/bin/env bash
# dev-up.sh — запустить все зависимости и сервисы локально.
#
# Режимы запуска:
#   bash scripts/dev-up.sh                  # без Keycloak (auth bypass)
#   WITH_KEYCLOAK=1 bash scripts/dev-up.sh  # с Keycloak (JWT validation)
#
# Переменные для режима WITH_KEYCLOAK (можно передать снаружи):
#   KEYCLOAK_CLIENT_SECRET  — client secret для oncall-api (обязателен)
#   KEYCLOAK_CLIENT_ID      — по умолчанию oncall-api
#   KEYCLOAK_REALM          — по умолчанию oncall
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVICES=(ingestion incident scheduling escalation notification)
PIDS=()

cleanup() {
  echo ""
  echo "==> Останавливаю сервисы..."
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
  echo "==> Останавливаю docker compose..."
  docker compose -f "$ROOT/docker-compose.yaml" down --remove-orphans
}
trap cleanup EXIT INT TERM

# ── 1. Зависимости ────────────────────────────────────────────────────────────
WITH_KEYCLOAK="${WITH_KEYCLOAK:-}"

if [[ -n "$WITH_KEYCLOAK" ]]; then
  echo "==> Запускаю PostgreSQL / Redis / RabbitMQ / Keycloak..."
  docker compose -f "$ROOT/docker-compose.yaml" up -d --wait postgres redis rabbitmq keycloak
else
  echo "==> Запускаю PostgreSQL / Redis / RabbitMQ..."
  docker compose -f "$ROOT/docker-compose.yaml" up -d --wait postgres redis rabbitmq
fi

echo "==> Жду готовности PostgreSQL..."
until docker compose -f "$ROOT/docker-compose.yaml" exec -T postgres \
    pg_isready -U oncall -q; do sleep 1; done

# ── 2. Сборка ─────────────────────────────────────────────────────────────────
echo "==> Собираю сервисы..."
for svc in "${SERVICES[@]}"; do
  echo "  go build $svc"
  (cd "$ROOT/services/$svc" && go build -o "/tmp/oncall-$svc" ./cmd/server) &
done
wait
echo "==> Сборка завершена."

# ── 3. Общие переменные окружения ─────────────────────────────────────────────
DB_DSN="postgres://oncall:oncall@localhost:5432/oncall?sslmode=disable"
RABBITMQ_URL="amqp://oncall:oncall@localhost:5672/"
REDIS_ADDR="localhost:6379"
ADMIN_API_KEY="${ADMIN_API_KEY:-devkey}"
LOG_LEVEL="${LOG_LEVEL:-info}"

# ── 4. Keycloak переменные (только при WITH_KEYCLOAK=1) ───────────────────────
KEYCLOAK_BASE="http://localhost:8090"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-oncall}"
KEYCLOAK_CLIENT_ID="${KEYCLOAK_CLIENT_ID:-oncall-api}"
KEYCLOAK_CLIENT_SECRET="${KEYCLOAK_CLIENT_SECRET:-}"
KEYCLOAK_JWKS_URL=""

if [[ -n "$WITH_KEYCLOAK" ]]; then
  if [[ -z "$KEYCLOAK_CLIENT_SECRET" ]]; then
    echo "ОШИБКА: WITH_KEYCLOAK=1 требует KEYCLOAK_CLIENT_SECRET" >&2
    exit 1
  fi
  KEYCLOAK_JWKS_URL="$KEYCLOAK_BASE/realms/$KEYCLOAK_REALM/protocol/openid-connect/certs"
  echo "==> Keycloak JWT включён: $KEYCLOAK_JWKS_URL"
else
  echo "==> Keycloak не используется (auth bypass через X-Admin-Key)"
fi

# ── 5. Запуск сервисов ────────────────────────────────────────────────────────
echo "==> Запускаю сервисы..."

# Бинарники запускаются из директории сервиса, чтобы file://./migrations резолвился корректно.

(cd "$ROOT/services/ingestion" && \
  HTTP_PORT=8080 DB_DSN="$DB_DSN" RABBITMQ_URL="$RABBITMQ_URL" \
  REDIS_ADDR="$REDIS_ADDR" ADMIN_API_KEY="$ADMIN_API_KEY" LOG_LEVEL="$LOG_LEVEL" \
  KEYCLOAK_JWKS_URL="$KEYCLOAK_JWKS_URL" \
  /tmp/oncall-ingestion) &
PIDS+=($!)

(cd "$ROOT/services/incident" && \
  HTTP_PORT=8081 DB_DSN="$DB_DSN" RABBITMQ_URL="$RABBITMQ_URL" \
  ADMIN_API_KEY="$ADMIN_API_KEY" LOG_LEVEL="$LOG_LEVEL" \
  KEYCLOAK_JWKS_URL="$KEYCLOAK_JWKS_URL" \
  /tmp/oncall-incident) &
PIDS+=($!)

(cd "$ROOT/services/scheduling" && \
  HTTP_PORT=8082 DB_DSN="$DB_DSN" RABBITMQ_URL="$RABBITMQ_URL" \
  REDIS_ADDR="$REDIS_ADDR" ADMIN_API_KEY="$ADMIN_API_KEY" LOG_LEVEL="$LOG_LEVEL" \
  KEYCLOAK_JWKS_URL="$KEYCLOAK_JWKS_URL" \
  KEYCLOAK_ADMIN_URL="$KEYCLOAK_BASE" \
  KEYCLOAK_REALM="$KEYCLOAK_REALM" \
  KEYCLOAK_CLIENT_ID="$KEYCLOAK_CLIENT_ID" \
  KEYCLOAK_CLIENT_SECRET="$KEYCLOAK_CLIENT_SECRET" \
  /tmp/oncall-scheduling) &
PIDS+=($!)

(cd "$ROOT/services/escalation" && \
  HTTP_PORT=8083 DB_DSN="$DB_DSN" RABBITMQ_URL="$RABBITMQ_URL" \
  ADMIN_API_KEY="$ADMIN_API_KEY" LOG_LEVEL="$LOG_LEVEL" \
  KEYCLOAK_JWKS_URL="$KEYCLOAK_JWKS_URL" \
  SCHEDULING_URL="http://localhost:8082" \
  SCHEDULING_ADMIN_KEY="$ADMIN_API_KEY" \
  /tmp/oncall-escalation) &
PIDS+=($!)

(cd "$ROOT/services/notification" && \
  HTTP_PORT=8084 DB_DSN="$DB_DSN" RABBITMQ_URL="$RABBITMQ_URL" \
  REDIS_ADDR="$REDIS_ADDR" ADMIN_API_KEY="$ADMIN_API_KEY" LOG_LEVEL="$LOG_LEVEL" \
  KEYCLOAK_JWKS_URL="$KEYCLOAK_JWKS_URL" \
  SCHEDULING_URL="http://localhost:8082" \
  SCHEDULING_ADMIN_KEY="$ADMIN_API_KEY" \
  /tmp/oncall-notification) &
PIDS+=($!)

echo ""
echo "==> Все сервисы запущены:"
echo "   ingestion:   http://localhost:8080"
echo "   incident:    http://localhost:8081"
echo "   scheduling:  http://localhost:8082"
echo "   escalation:  http://localhost:8083"
echo "   notification:http://localhost:8084"
if [[ -n "$WITH_KEYCLOAK" ]]; then
  echo "   keycloak:    $KEYCLOAK_BASE  (realm: $KEYCLOAK_REALM)"
fi
echo ""
echo "   ADMIN_API_KEY=$ADMIN_API_KEY"
echo ""
echo "Нажмите Ctrl+C для остановки."
wait
