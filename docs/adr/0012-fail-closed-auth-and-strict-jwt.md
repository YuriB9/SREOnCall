# ADR-0012: Fail-closed аутентификация и строгая валидация JWT

- Status: Accepted
- Date: 2026-06-13
- Change: harden-auth-validation
- Affected: pkg/auth, services/incident, services/escalation, services/notification, services/scheduling, deploy/k8s

## Context

Аудит безопасности (`docs/audit/05-security.md`) выявил на главном trust boundary платформы:

- **S1 (fail-open):** при пустом `KEYCLOAK_JWKS_URL` каждый из 4 сервисов с JWT-middleware подключал проходной обработчик — при мисконфиге (опечатка в манифесте, незаполненный secret) сервис анонимно обслуживал все защищённые маршруты.
- **S3:** admin-key сравнивался через `==` (не constant-time, CWE-208).
- **S4:** `jwt.ParseWithClaims` вызывался без опций — не проверялись `aud`/`iss`, allowlist алгоритмов и наличие `exp`.
- **S5:** схема `KEYCLOAK_JWKS_URL` не форсилась на https.

Принцип `golang-security` — «fail closed, never open». Решение фиксирует новую security-постуру auth, дополняя ADR-0006 (Keycloak/JWKS-группы) и ADR-0009 (X-Admin-Key как сервисный механизм).

## Options considered

- **Fail-closed с явным escape-hatch `AUTH_DISABLED`** — обязательный JWKS на старте, отключение auth только осознанным флагом для локалки с громким `Warn`. Принято: закрывает S1, сохраняет локальную разработку.
- **Оставить fail-open, но логировать предупреждение** — не закрывает риск анонимного доступа при мисконфиге. Отклонено.
- **`iss`/`aud` обязательны всегда** — корректнее по best practice, но жёсткое требование уронило бы деплои, ещё не прописавшие эти env. Отклонено в пользу enforce-if-configured с предупреждением; ужесточение — отдельный шаг после раскатки конфигурации.
- **Хелпер wiring в общий `pkg/httpserver`** — устранил бы дублирование fail-closed-блока в 4 `main.go` и риск «забыть» защиту в новом сервисе. Отложено: это F10/CH10, вне объёма CH03; блоки сделаны единообразными для последующего выноса.

## Decision

- **`pkg/auth.Middleware` принимает `Options`** (`JWKSURL`, `AdminKey`, `Issuer`, `Audience`, `AllowInsecureJWKS`) вместо двух позиционных аргументов.
- **S1:** при пустом `KEYCLOAK_JWKS_URL` сервис делает `os.Exit(1)`. Проходной обработчик допускается только при `AUTH_DISABLED=true` с `logger.Warn`.
- **S3:** admin-key сравнивается через `subtle.ConstantTimeCompare` с предварительной проверкой непустоты обеих сторон.
- **S4:** парсер всегда получает `jwt.WithValidMethods(["RS256"])` и `jwt.WithExpirationRequired()`; `jwt.WithIssuer`/`jwt.WithAudience` — когда заданы `KEYCLOAK_ISSUER`/`KEYCLOAK_AUDIENCE` (иначе `Warn` на старте о неполной валидации).
- **S5:** схема JWKS URL обязана быть `https`; `http` допускается только при `AUTH_INSECURE=true` (in-cluster/локалка). Проверка — внутри `Middleware`, ошибка фатальна для сервиса.

## Consequences

- **Деплой с пустым `KEYCLOAK_JWKS_URL` без `AUTH_DISABLED` больше не стартует** — это цель S1. Перед выкатом убедиться, что у всех prod-сервисов JWKS задан.
- **Локальный запуск Go-сервисов** (вне docker-compose, где крутится только инфра) без Keycloak требует `AUTH_DISABLED=true`.
- **In-cluster JWKS по http** (текущие k8s-configmap) требует `AUTH_INSECURE=true` — флаг проставлен в манифестах с пометкой перевести на https.
- **Полная валидация `iss`/`aud` пока не включена** в манифестах (закомментированные примеры) — включается отдельным шагом после сверки realm/audience; до этого валидация по iss/aud не строже прежней.
- **Admin-key остаётся god-key** (кросс-тенант обход) — сужение полномочий/ротация/переход на client-credentials зафиксированы как follow-up (см. S3, ADR-0009), вне CH03.
- **Токен без `exp` теперь отклоняется** — у Keycloak access-token `exp` присутствует всегда, риск для штатных токенов отсутствует.
