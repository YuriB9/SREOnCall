## Why

Аудит безопасности (`docs/audit/05-security.md`) выявил ослабленную аутентификацию на главном trust boundary платформы:

- **S1 (major, fail-open):** при пустом `KEYCLOAK_JWKS_URL` middleware заменяется на проходной обработчик — сервис обслуживает все защищённые маршруты без аутентификации. Триггерится мисконфигом (опечатка в манифесте, незаполненный secret).
- **S3 (major):** admin-key сравнивается через `==` (не constant-time, CWE-208) и даёт неограниченный кросс-тенант god-доступ.
- **S4 (major):** JWT валидируется без проверки `aud`/`iss`, без allowlist алгоритмов и без требования `exp` — токен другого клиента realm или токен без срока годности проходит.
- **S5 (minor):** схема `KEYCLOAK_JWKS_URL` не форсится на https — ключи JWKS можно подменить MITM.

Принцип `golang-security` — «fail closed, never open». Сейчас нарушается; чиним до раскатки остальных фаз (S6/rate-limit → CH10, S2/SSRF → CH04 вне объёма).

## What Changes

- **S1 — fail-closed аутентификация.** Пустой `KEYCLOAK_JWKS_URL` на старте → `logger.Error` + `os.Exit(1)`. Отключить auth можно только явным `AUTH_DISABLED=true` (локальная разработка) с громким `logger.Warn`. Затрагивает 4 сервиса с JWT-middleware.
- **S3 — constant-time сравнение admin-key.** `subtle.ConstantTimeCompare` вместо `==` (с предварительной проверкой непустоты обеих сторон).
- **S4 — строгая валидация JWT.** В парсер добавляются `jwt.WithValidMethods(["RS256"])` и `jwt.WithExpirationRequired()` (всегда), а также `jwt.WithIssuer(...)`/`jwt.WithAudience(...)` — когда заданы `KEYCLOAK_ISSUER`/`KEYCLOAK_AUDIENCE`. Если они не заданы — `logger.Warn` на старте о неполной валидации (enforce-if-configured: не ломаем существующие деплои, ужесточение iss/aud — отдельным шагом после раскатки env).
- **S5 — форс https для JWKS URL.** Схема обязана быть `https`; `http` допускается только при `AUTH_INSECURE=true` (локалка).
- **Рефактор сигнатуры `pkg/auth.Middleware`.** `Middleware(jwksURL, adminKey string)` → `Middleware(Options)` со полями `JWKSURL`, `AdminKey`, `Issuer`, `Audience`, `AllowInsecureJWKS`. Это внутренняя Go-сигнатура `pkg/auth`, меняется вместе с 4 вызывающими; HTTP-API и события шины не затрагиваются — **не BREAKING** для внешних контрактов.

Новые переменные окружения (миграция конфигурации, не код): `AUTH_DISABLED`, `KEYCLOAK_ISSUER`, `KEYCLOAK_AUDIENCE`, `AUTH_INSECURE`.

## Capabilities

### New Capabilities
<!-- Новых capability не вводится. -->

### Modified Capabilities
<!-- Дельта-спека нет: наблюдаемое поведение продуктовых capability не меняется. Это инфраструктурное укрепление pkg/auth и wiring сервисов (прецедент harden-auth-shell — backend без дельты). Гарантии auth фиксируются в Impact и ADR-0012. -->

## Impact

- **Затронутые сервисы:** `incident`, `escalation`, `notification`, `scheduling` — `cmd/server/main.go` (fail-closed wiring) и `internal/config/config.go` (новые поля/env). `ingestion` использует webhook-token аутентификацию, JWT-middleware не подключает — **вне объёма**.
- **Общий код:** `pkg/auth/auth.go` (Options, constant-time, опции парсера, проверка https).
- **События RabbitMQ:** не затрагиваются.
- **HTTP-API:** контракты эндпоинтов не меняются; меняется только поведение при мисконфиге (раньше — анонимный доступ, теперь — отказ старта).
- **Деплой:** `deploy/k8s` и `docker-compose` — добавить новые env (`AUTH_DISABLED`/`KEYCLOAK_ISSUER`/`KEYCLOAK_AUDIENCE`/`AUTH_INSECURE`). Деплои с непустым `KEYCLOAK_JWKS_URL` стартуют как раньше (iss/aud — с предупреждением, пока не сконфигурированы); деплои с пустым JWKS без `AUTH_DISABLED` перестают стартовать — это цель S1.
- **Документация:** ADR-0012 (fail-closed + строгая валидация JWT), обновление `docs/spec-vs-code-audit.md` (S1/S3/S4/S5).
- **Тесты:** юнит-тесты `pkg/auth` (constant-time, отклонение токена без `exp`/чужого `aud`/`iss`/не-RS256, отказ http-JWKS без флага).
