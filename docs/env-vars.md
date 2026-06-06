# Переменные окружения сервисов

Обозначения: **Secret** — хранить в Kubernetes Secret; **ConfigMap** — в ConfigMap.

---

## ingestion (порт 8080)

| Переменная | По умолчанию | K8s | Описание |
|---|---|---|---|
| `HTTP_PORT` | `8080` | ConfigMap | Порт HTTP-сервера |
| `LOG_LEVEL` | `info` | ConfigMap | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| `DB_DSN` | — | **Secret** | DSN PostgreSQL, например `postgres://user:pass@host:5432/db?sslmode=disable` |
| `REDIS_ADDR` | `localhost:6379` | ConfigMap | Адрес Redis |
| `REDIS_PASSWORD` | — | **Secret** | Пароль Redis |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | **Secret** | URL RabbitMQ |
| `DEDUP_TTL_SECONDS` | `14400` (4 ч) | ConfigMap | TTL ключа дедупликации в Redis |

---

## incident (порт 8081)

| Переменная | По умолчанию | K8s | Описание |
|---|---|---|---|
| `HTTP_PORT` | `8081` | ConfigMap | Порт HTTP-сервера |
| `LOG_LEVEL` | `info` | ConfigMap | Уровень логирования |
| `DB_DSN` | — | **Secret** | DSN PostgreSQL |
| `RABBITMQ_URL` | — | **Secret** | URL RabbitMQ (обязателен) |
| `ADMIN_API_KEY` | — | **Secret** | Ключ для bypass аутентификации через `X-Admin-Key` |
| `KEYCLOAK_JWKS_URL` | — | ConfigMap | URL JWKS эндпоинта Keycloak, например `http://keycloak:8080/realms/oncall/protocol/openid-connect/certs` |

---

## scheduling (порт 8082)

| Переменная | По умолчанию | K8s | Описание |
|---|---|---|---|
| `HTTP_PORT` | `8082` | ConfigMap | Порт HTTP-сервера |
| `DB_DSN` | — | **Secret** | DSN PostgreSQL |
| `ADMIN_API_KEY` | — | **Secret** | Ключ для bypass аутентификации |
| `KEYCLOAK_JWKS_URL` | — | ConfigMap | URL JWKS эндпоинта Keycloak |
| `REDIS_ADDR` | `localhost:6379` | ConfigMap | Адрес Redis (для webhook token index) |
| `REDIS_PASSWORD` | — | **Secret** | Пароль Redis |
| `KEYCLOAK_ADMIN_URL` | `http://localhost:8080` | ConfigMap | Базовый URL Keycloak Admin API |
| `KEYCLOAK_REALM` | `oncall` | ConfigMap | Realm Keycloak |
| `KEYCLOAK_CLIENT_ID` | — | **Secret** | Client ID для чтения групп через Admin API |
| `KEYCLOAK_CLIENT_SECRET` | — | **Secret** | Client Secret для чтения групп |

> Если `KEYCLOAK_CLIENT_ID` или `KEYCLOAK_CLIENT_SECRET` не заданы, эндпоинт `GET /tenants/{slug}/members` вернёт `503`.

---

## escalation (порт 8083)

| Переменная | По умолчанию | K8s | Описание |
|---|---|---|---|
| `HTTP_PORT` | `8083` | ConfigMap | Порт HTTP-сервера |
| `LOG_LEVEL` | `info` | ConfigMap | Уровень логирования |
| `DB_DSN` | — | **Secret** | DSN PostgreSQL |
| `RABBITMQ_URL` | — | **Secret** | URL RabbitMQ (необязателен; без него AMQP-консьюмер не запускается) |
| `ADMIN_API_KEY` | — | **Secret** | Ключ для bypass аутентификации |
| `KEYCLOAK_JWKS_URL` | — | ConfigMap | URL JWKS эндпоинта Keycloak |
| `SCHEDULING_URL` | `http://localhost:8082` | ConfigMap | Базовый URL scheduling-сервиса |

---

## notification (порт 8084)

| Переменная | По умолчанию | K8s | Описание |
|---|---|---|---|
| `HTTP_PORT` | `8084` | ConfigMap | Порт HTTP-сервера |
| `LOG_LEVEL` | `info` | ConfigMap | Уровень логирования |
| `DB_DSN` | — | **Secret** | DSN PostgreSQL |
| `RABBITMQ_URL` | — | **Secret** | URL RabbitMQ |
| `REDIS_ADDR` | `localhost:6379` | ConfigMap | Адрес Redis (rate limiter) |
| `REDIS_PASSWORD` | — | **Secret** | Пароль Redis |
| `ADMIN_API_KEY` | — | **Secret** | Ключ для bypass аутентификации |
| `KEYCLOAK_JWKS_URL` | — | ConfigMap | URL JWKS эндпоинта Keycloak |
| `SCHEDULING_URL` | `http://localhost:8082` | ConfigMap | URL scheduling-сервиса (для получения tenant notification config) |
| `SMTP_HOST` | `localhost` | ConfigMap | SMTP-хост |
| `SMTP_PORT` | `25` | ConfigMap | SMTP-порт |
| `SMTP_USERNAME` | — | **Secret** | Логин SMTP |
| `SMTP_PASSWORD` | — | **Secret** | Пароль SMTP |
| `SMTP_FROM` | `oncall@example.com` | ConfigMap | Адрес отправителя |
| `RATE_LIMIT_MAX` | `5` | ConfigMap | Максимум уведомлений на контакт за окно |
| `RATE_LIMIT_WINDOW_SECONDS` | `600` | ConfigMap | Окно rate limiter в секундах |

---

## Пример Kubernetes Secret (все сервисы)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: oncall-secrets
  namespace: oncall
type: Opaque
stringData:
  DB_DSN: "postgres://oncall:CHANGEME@postgres:5432/oncall?sslmode=disable"
  RABBITMQ_URL: "amqp://oncall:CHANGEME@rabbitmq:5672/"
  REDIS_PASSWORD: "CHANGEME"
  ADMIN_API_KEY: "CHANGEME"
  SMTP_USERNAME: "oncall@example.com"
  SMTP_PASSWORD: "CHANGEME"
  KEYCLOAK_CLIENT_ID: "oncall-api"
  KEYCLOAK_CLIENT_SECRET: "CHANGEME"
```

## Пример Kubernetes ConfigMap (scheduling)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: scheduling-config
  namespace: oncall
data:
  HTTP_PORT: "8082"
  LOG_LEVEL: "info"
  REDIS_ADDR: "redis:6379"
  KEYCLOAK_JWKS_URL: "http://keycloak:8080/realms/oncall/protocol/openid-connect/certs"
  KEYCLOAK_ADMIN_URL: "http://keycloak:8080"
  KEYCLOAK_REALM: "oncall"
```
