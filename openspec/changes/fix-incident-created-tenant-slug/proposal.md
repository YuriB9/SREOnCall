## Why

Consumer incident-сервиса не заполняет `tenant_slug` при создании инцидента из алерта: в БД пишется значение по умолчанию `''`, и событие `incident.created` уходит с пустым `tenant_slug`. Из-за этого ломается основной (автоматический) поток платформы: escalation при авто-назначении политики сохраняет пустой slug и не может разрезолвить дежурного (`GET /api/schedules/v1//schedules/{id}/oncall` → ошибка), а notification не получает per-tenant конфигурацию Mattermost (`GET /api/schedules/v1/tenants//notification-config` → 404) и строит битую ссылку на инцидент (`{base}//incidents?...`). Дежурные не получают персональных уведомлений по каждому авто-созданному инциденту. Обнаружено при сверке спецификаций с кодом (`docs/spec-vs-code-audit.md`, пункт 1).

## What Changes

- Consumer incident-сервиса заполняет `tenant_slug` при создании инцидента из алерта. В текущей архитектуре `tenant_id` алерта и есть slug тенанта (Redis-индекс вебхук-токенов хранит slug), поэтому slug берётся из `tenant_id` события.
- Событие `incident.created` (и `incident.updated` из consumer-пути авто-резолва) гарантированно содержит непустой `tenant_slug`.
- Защитный fallback в escalation: при потреблении `incident.created` с пустым `tenant_slug` использовать `tenant_id` (симметрично уже существующему fallback в ручном пути привязки политики).
- Backfill существующих записей: инциденты с пустым `tenant_slug` обновляются значением `tenant_id` (миграция); активные состояния эскалации с пустым `tenant_slug` — аналогично.
- E2E/интеграционное покрытие: тест, проверяющий, что `incident.created` содержит непустой `tenant_slug` и что цепочка авто-эскалации резолвит дежурного.

## Capabilities

### New Capabilities

Нет — новые возможности не вводятся.

### Modified Capabilities

- `incident-management`: требование к событию `incident.created` уточняется — `tenant_slug` ДОЛЖЕН быть непустым и для инцидентов, созданных consumer'ом из алертов (не только в HTTP-пути); событие `incident.updated`, публикуемое consumer'ом при авто-резолве, также содержит непустой `tenant_slug`.
- `escalation-policies`: при авто-назначении политики из `incident.created` сервис ДОЛЖЕН использовать `tenant_id` как fallback для пустого `tenant_slug`, чтобы резолв дежурного и события `escalation.triggered` оставались работоспособными при событиях от старых версий incident-сервиса.

## Impact

- `services/incident/internal/consumer/consumer.go` — заполнение `TenantSlug` при создании инцидента (handleFiring).
- `services/incident/migrations/` — новая миграция backfill `tenant_slug = tenant_id WHERE tenant_slug = ''`.
- `services/escalation/internal/consumer/consumer.go` (или `escalator.AutoAssign`) — fallback `tenant_slug → tenant_id`.
- `services/escalation/migrations/` — backfill `tenant_slug` в `escalation_states` (активные записи).
- Тесты: интеграционный тест consumer'а incident (поле события), интеграционный тест escalation (fallback), e2e-сценарий авто-эскалации (опционально, при наличии инфраструктуры).
- Нижестоящие сервисы (notification, frontend-ссылки) изменений не требуют — начинают получать корректный slug.
