## Why

Спецификация `incident-management` уже требует, чтобы «лейблы алерта были доступны на вкладке алертов инцидента» (требование «Лейблы инцидента»), но реализация этого не делает: таблица `incident.incident_alerts` не хранит лейблы, доменный `IncidentAlert` и ответ `GET .../alerts` их не содержат. Лейблы исходного алерта мёржатся только в лейблы инцидента при его создании, а per-alert лейблы отбрасываются при привязке. В результате в UI алерты одного инцидента невозможно различить осмысленно: `source` и `group_key` у них совпадают (по `group_key` они и сгруппированы), а `fingerprint` — нечитаемый хеш. Ранее UI показывал `instance` инцидента на каждом алерте, что давало одинаковое значение для всех (баг).

## What Changes

- В таблицу `incident.incident_alerts` добавляется колонка `labels jsonb NOT NULL DEFAULT '{}'` (миграция `000004`).
- Доменная модель `IncidentAlert` получает поле `Labels map[string]string`; ответ `GET /api/incidents/v1/{tenant}/incidents/{incidentId}/alerts` возвращает `labels` по каждому алерту (аддитивно).
- Консьюмер при автоматической привязке сохраняет лейблы алерта (`alert.Labels` уже доступны в момент привязки) в `incident_alerts.labels`.
- `store.AttachAlert` и запрос `ListIncidentAlerts` пишут/читают колонку `labels`.
- Фронтенд показывает на каждом алерте различающие лейблы — те, что не входят в общий (grouping) набор инцидента; в первую очередь `instance`.
- Ручная привязка (`POST .../alerts`) сохраняет пустой набор лейблов, если они не переданы (поведение остаётся валидным).

Изменения аддитивны к контракту API: добавляется поле `labels`, существующие поля не меняются.

## Capabilities

### New Capabilities
<!-- нет новых capability -->

### Modified Capabilities
- `incident-management`: требование «Детали инцидента и его алерты» уточняется — ответ списка алертов ДОЛЖЕН включать лейблы каждого алерта; согласуется с уже существующим требованием о доступности лейблов алерта на вкладке.

## Impact

- **Сервисы:** `services/incident` — миграция `000004`, `internal/domain` (`IncidentAlert.Labels`), `internal/store` (`AttachAlert`, `ListIncidentAlerts`), `internal/consumer` (передача `alert.Labels`), `internal/handler` (ответ `/alerts`).
- **БД:** новая колонка `incident.incident_alerts.labels jsonb` (с дефолтом — существующие строки получают `{}`).
- **API:** `GET .../incidents/{id}/alerts` возвращает дополнительное поле `labels` (аддитивно, не BREAKING).
- **Фронтенд:** тип `Alert` (`frontend/src/api/types.ts`) и рендер вкладки «Алерты» в `IncidentDetailPanel.tsx`.
- **События RabbitMQ:** не затрагиваются (лейблы уже присутствуют в потребляемом сообщении алерта).
