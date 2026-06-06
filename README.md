# SRE OnCall Platform

Go-монорепо с пятью микросервисами для управления дежурствами, инцидентами и уведомлениями.

## Архитектура

```text
Alertmanager / Grafana / Zabbix
         │ webhook
         ▼
    ┌─────────────┐   AMQP alerts      ┌──────────────┐   AMQP incidents   ┌─────────────┐
    │  ingestion  │ ─────────────────► │   incident   │ ─────────────────► │  escalation │
    │  :8080      │                    │   :8081      │                    │  :8083      │
    └─────────────┘                    └──────────────┘                    └─────────────┘
                                                                                  │ AMQP escalations
    ┌─────────────┐                                                               ▼
    │  scheduling │ ◄── GET /oncall ──────────────────────────────────── ┌─────────────────┐
    │  :8082      │                                                      │  notification   │
    └─────────────┘                                                      │  :8084          │
                                                                         └─────────────────┘
Хранилища: PostgreSQL · Redis · RabbitMQ · Keycloak (OIDC)
```

## Быстрый старт (локальная разработка)

### Требования

- Go 1.22+
- Docker + Docker Compose v2

### Запуск

```bash
# Запустить все зависимости + сервисы
WITH_KEYCLOAK=1 \
KEYCLOAK_CLIENT_SECRET=dev-secret-oncall \
bash scripts/dev-up.sh

```

Скрипт:

1. Поднимает PostgreSQL, Redis, RabbitMQ, Keycloak через `docker compose`
2. Собирает все 5 сервисов
3. Запускает их с правильными переменными окружения

### Проверка

```bash
# Создать тенант
curl -X POST http://localhost:8082/api/schedules/v1/tenants \
  -H "X-Admin-Key: devkey" \
  -H "Content-Type: application/json" \
  -d '{"slug":"team-a","name":"Team Alpha"}'

# Создать вебхук-токен
curl -X POST http://localhost:8082/api/schedules/v1/tenants/team-a/webhook-tokens \
  -H "X-Admin-Key: devkey" \
  -H "Content-Type: application/json" \
  -d '{"source":"alertmanager"}'

# Healthcheck всех сервисов
for port in 8080 8081 8082 8083 8084; do
  echo -n "port $port: "; curl -sf http://localhost:$port/healthz && echo OK
done
```

### Метрики Prometheus

Каждый сервис отдаёт метрики на `/metrics`:

```bash
curl http://localhost:8082/metrics
```

## Тесты

```bash
# Unit-тесты
go test github.com/sre-oncall/scheduling/internal/rotation/...

# Integration-тесты (in-memory, без зависимостей)
go test -tags integration github.com/sre-oncall/scheduling/internal/handler/...
go test -tags integration github.com/sre-oncall/incident/internal/handler/...
go test -tags integration github.com/sre-oncall/escalation/internal/handler/...

# E2E-тесты (требуют запущенных сервисов)
SCHEDULING_URL=http://localhost:8082 \
INGESTION_URL=http://localhost:8080 \
INCIDENT_URL=http://localhost:8081 \
ADMIN_API_KEY=devkey \
go test -tags e2e -v ./tests/e2e/...
```

## Развёртывание в Kubernetes (k3s)

### Манифесты

Kubernetes-манифесты находятся в `deploy/k8s/<service>/`:

- `deployment.yaml` — Deployment с readiness/liveness probe на `/healthz` и `/readyz`
- `service.yaml` — ClusterIP Service
- `configmap.yaml` — ConfigMap с нечувствительными env vars
- `secret.yaml` — Secret с паролями и ключами
- `deploy/k8s/ingress.yaml` — Ingress на `oncall.local`

### Применить все манифесты

```bash
# Namespace
kubectl create namespace oncall

# Для каждого сервиса
for svc in ingestion incident scheduling escalation notification; do
  kubectl apply -n oncall -f deploy/k8s/$svc/
done
kubectl apply -n oncall -f deploy/k8s/ingress.yaml
```

### Настройка Keycloak

1. Создать realm `oncall`
2. Создать client `oncall-api` с `confidential` access type
3. Создать группы: `/{tenant-slug}` и `/{tenant-slug}/admins`
4. Задать env vars `KEYCLOAK_JWKS_URL` и `KEYCLOAK_CLIENT_ID/SECRET`

## Мониторинг

- **Prometheus**: scrape `/metrics` на каждом сервисе
- **RabbitMQ**: включить плагин `rabbitmq_prometheus` для метрик очередей
- **Grafana**: дашборды строить на метриках `http_requests_total`, `http_request_duration_seconds`, `ingestion_dedup_hits_total`

## Структура монорепо

```text
services/
  ingestion/      — приём вебхуков, дедупликация, публикация в RabbitMQ
  incident/       — управление инцидентами, группировка алертов
  scheduling/     — расписания, ротации, управление тенантами
  escalation/     — политики эскалации, переходы уровней
  notification/   — email и Mattermost уведомления
pkg/
  amqp/           — обёртка RabbitMQ
  auth/           — JWT middleware (JWKS), tenant checks
  db/             — pgxpool helper
  domain/         — общие типы (Alert)
  logger/         — structured slog
  metrics/        — Prometheus middleware
  migrate/        — golang-migrate helper
  redis/          — redis client helper
```
