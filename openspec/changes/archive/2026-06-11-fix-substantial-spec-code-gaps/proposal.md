## Why

Сверка спецификаций с кодом (`docs/spec-vs-code-audit.md`, раздел «Существенные расхождения», пункты 3–7) выявила пять расхождений, каждое из которых либо делает заявленное спекой поведение неверным, либо ломает видимую пользователю функциональность: TTL дедупликации в коде в 48 раз больше заявленного; правила группировки для Alertmanager никогда не применяются из-за разных имён источника; фильтры критичности «Средний»/«Низкий» в UI никогда ничего не находят, а инциденты `warning`/`info` рендерятся без бейджа; звуковые уведомления включены по умолчанию вопреки спеке; при конфликте переопределений UI показывает «Invalid Date — Invalid Date (undefined)» вместо описания конфликта.

## What Changes

- **(3) TTL дедупликации** — спека выравнивается по коду: значение по умолчанию фиксируется как **4 часа** (соответствует типичному `repeat_interval` Alertmanager; resolved-алерт сбрасывает ключ, поэтому повторное срабатывание после резолва проходит сразу). Код не меняется.
- **(4) Source алертов Alertmanager** — канонизируется значение `alertmanager`: ingestion перестаёт писать `prometheus` (`SourcePrometheus` → `SourceAlertmanager` = `"alertmanager"`), backfill-миграции обновляют `source` в исторических данных (`ingestion.raw_alerts`, `incident.incident_alerts`); поиск правил группировки в incident-сервисе принимает `prometheus` как алиас `alertmanager` на переходный период (сообщения старого формата в очереди). **Известное ограничение:** fingerprint включает source, поэтому firing-алерты, принятые до деплоя, не сматчатся со своими resolved после деплоя — такие инциденты закрываются вручную.
- **(5) Словарь severity** — фронтенд приводится к словарю бэкенда `critical | high | warning | info` (типы, фильтр, подписи и стили бейджей: «Предупреждение», «Инфо»); спека `incident-dashboard-ui` фиксирует перечень значений.
- **(6) Звук по умолчанию выключен** — код приводится к спеке: при отсутствии ключа `oncall.audioEnabled` в `localStorage` звуковые уведомления выключены. Спека не меняется.
- **(7) Детали конфликта в 409** — scheduling включает в тело 409 поля `existing_start`, `existing_end`, `existing_user` конфликтующего переопределения (контракт, который фронтенд уже ожидает); спека `oncall-scheduling` фиксирует формат тела.

## Capabilities

### New Capabilities

Нет — новые возможности не вводятся.

### Modified Capabilities

- `alert-ingestion`: (3) TTL дедупликации по умолчанию — 4 часа вместо 5 минут; (4) каноническая схема алерта фиксирует значения `source`: `alertmanager` для вебхуков Alertmanager, `grafana` для Grafana.
- `incident-management`: (4) обработка алертов со старым source `prometheus` — алиас `alertmanager` при поиске правил группировки.
- `incident-dashboard-ui`: (5) перечень значений критичности в фильтре и бейджах — `critical | high | warning | info`, согласованный с канонической схемой алерта.
- `oncall-scheduling`: (7) тело ответа 409 при пересечении переопределений содержит `existing_start`, `existing_end`, `existing_user`.

## Impact

- `openspec/specs/alert-ingestion/spec.md` — обновление через delta (TTL, source).
- `services/ingestion/internal/...` — переименование константы `SourcePrometheus` → `SourceAlertmanager` (значение `alertmanager`) в `pkg/domain/alert.go` и местах использования; миграция backfill `raw_alerts.source`.
- `services/incident/internal/domain/incident.go`, `internal/consumer/consumer.go` — алиас `prometheus → alertmanager` при поиске правил; миграция backfill `incident_alerts.source`.
- `frontend/src/api/types.ts`, `frontend/src/pages/IncidentListPage.tsx`, `IncidentDetailPanel.tsx` — словарь severity, подписи, стили, фильтр.
- `frontend/src/hooks/useAudioEnabled.ts` — дефолт «выключено».
- `services/scheduling/internal/store/store.go`, `internal/handler/handler.go` — возврат конфликтующего переопределения из `CreateOverride`, структурированное тело 409.
- Тесты: ingestion (source/normalize), incident (алиас правил), scheduling (тело 409), frontend (severity, дефолт звука).
- `docs/spec-vs-code-audit.md` — пометка пунктов 3–7 как исправленных.
