## Why

Аудит области 8 (`docs/audit/08-testing.md`) показал: при 5161 строке тестов реальная защита dev-loop тоньше, чем кажется. CI-гейт и снятие лишнего тега `integration` уже закрыты в CH01 (T1, T2), но осталось три пробела, из-за которых регрессы конкурентности и переходов состояний (C1/C2, D1) проходят мимо тестов:

- **T3** — нигде нет `goleak`: утечки горутин в `consumer`/`monitor`/`pkg/amqp` тестами не ловятся.
- **T4** — дешёвые и важные юниты без покрытия: контракт `pkg/amqp.Envelope`, store эскалации (регресс-гард D1), `monitor`, notification `ratelimit`/`dispatcher`, `scheduling/keycloak`.
- **T5** — нет `t.Parallel()`, а матричные кейсы (парсер длительностей, матрица переходов) написаны не table-driven; плюс латентный `go vet` httpresponse-nil-deref в `*/handler_test.go` (backlog T5/CH17).

## What Changes

- **T3 — детекция утечек горутин.** `TestMain` с `goleak.VerifyTestMain(m)` в пакетах с фоновыми горутинами: `pkg/amqp`, консьюмеры incident/escalation/notification, escalation `monitor`. В `tenantcache` подключить реальный `goleak` на sweeper-горутину (закрыть хвост CH16). Новая dev-зависимость `go.uber.org/goleak`.
- **T4 — недостающие юниты/integration-тесты:**
  - `pkg/amqp/envelope_test.go` — round-trip `Wrap`/`Unwrap`, версия конверта, ошибка на битом payload, поведение при битом/неизвестном payload.
  - `escalation/store` — настоящий integration-тест (`//go:build integration`, Postgres из docker-compose, skip без `DB_DSN`): регресс-гард **D1** — CAS-конфликт `AdvanceEscalationState` под параллелизмом (нет двойной эскалации), `ListExpiredStates` с `FOR UPDATE SKIP LOCKED`.
  - `escalation/monitor` — юнит на `step()` поверх мок-Store/escalator (без Postgres).
  - notification `ratelimit` — юнит token-bucket (S6, security-relevant).
  - notification `dispatcher` (email/mattermost) — отменяемое ожидание ретрая по `ctx` (регресс C5).
  - `scheduling/keycloak/client` — клиент Admin API через `httptest`.
- **T5 — параллелизм и table-driven:**
  - `ParseISO8601Duration` (rotation) → table-driven с именованными подтестами + `t.Parallel()`.
  - матрица переходов стейт-машины incident → именованные подтесты + `t.Parallel()`.
  - `t.Parallel()` в независимые юнит-тесты, где его нет.
  - починить `go vet` httpresponse-nil-deref в `handler_test.go` (incident/escalation/scheduling): `resp` используется до проверки `err`.

## Capabilities

### New Capabilities
<!-- Нет: чейндж не вводит наблюдаемого поведения продуктовой capability. -->

### Modified Capabilities
<!-- Нет. Чейндж infra/tests: меняется только тестовый код и dev-зависимости,
     гарантии продуктовых capability не затрагиваются. Архив с --skip-specs
     (прецедент harden-auth-shell). -->

## Impact

- **Затронутые сервисы (только тестовый код и dev-зависимости):** ingestion, incident, escalation, notification, scheduling и общий модуль `pkg` (`pkg/amqp`).
- **Продакшн-код не меняется.** API, схема БД и **события RabbitMQ** (`alert.received`, `incident.created|updated`, `escalation.triggered|exhausted`) — без изменений. **Не BREAKING.**
- **Новая dev-зависимость** `go.uber.org/goleak` в модулях `pkg`, `incident`, `escalation`, `notification` (`go mod tidy` в каждом).
- **CI:** дефолтный прогон обогащается goleak-проверками и новыми юнитами; integration-джоб (Postgres) получает регресс-тест escalation store. Линтер `paralleltest` (в baseline с CH01) подсветит новые `t.Parallel()`.
- **Closes:** T3, T4, T5 из `docs/audit/08-testing.md`.
