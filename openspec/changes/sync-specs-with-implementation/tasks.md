# Задачи: Синхронизация спецификаций с реализацией

Изменение документационное: кодовых правок нет, задачи — построчная верификация дельт против кода перед архивацией.

## 1. Верификация бэкенд-дельт против кода

- [x] 1.1 `alert-ingestion`: пути `/api/ingest/v1/webhook/{alertmanager,grafana}` (`ingestion/cmd/server/main.go`), Redis-lookup `oncall:tokens:{hash}`, 3 ретрая публикации (`pkg/amqp`), 503 и откат dedup-ключа (`handler.go:77-84`), маппинг `state: ok|paused → resolved` (`grafana.go`)
- [x] 1.2 `escalation-policies`: модель политики (`name`, `tiers[].tier_number/timeout_seconds/notify_schedule_id`, отсутствие `schedule_id` и `PUT /policies/{id}`), немедленный tier-1 (`escalator.go AssignPolicy`), остановка на `acknowledged` и `resolved` (`consumer.go`), `GET /default-policy` и `GET /incidents/{id}/history` (маршруты `cmd/server/main.go`)
- [x] 1.3 `incident-management`: severity/title от первого алерта без пересчёта, лейблы только при создании, осиротевший resolved → ack без ошибки (`consumer.go`), фильтры списка `status` (CSV после P0), `severity`, `from_time`, `to_time`, `cursor`/`limit` без `label` (`store.go`), `GET /incidents/{id}` и `/alerts`
- [x] 1.4 `tenant-management`: `POST /tenants` доступен любому аутентифицированному (`main.go:107`), `DeleteTenant` каскадно удаляет только данные scheduling (`store.go:256`)

## 2. Верификация UI-дельт против кода

- [x] 2.1 `oncall-schedule-ui`: per-schedule карточки `OnCallCard` с поллингом 60 с, Gantt через `GET /shifts?from&to` одним запросом на расписание, мобильный список <640px (`SchedulesPage.tsx`)
- [x] 2.2 `user-profile-ui`: эндпоинты `/api/notifications/v1/{tenant}/contacts/{userId}`, селектор тенанта при нескольких командах, 404 → пустая форма (`ProfilePage.tsx`, `api/profile.ts`)
- [x] 2.3 `escalation-policy-ui`: сохранение заменой POST→default→DELETE (`api/escalations.ts useReplacePolicy`), `PUT /default-policy`, `timeout_seconds` (`types.ts`)
- [x] 2.4 `tenant-settings-ui`: пути `/tenants/{tenant}/webhook-tokens|members`, выбор источника из enum (`TenantSettingsPage.tsx VALID_SOURCES` — после P1 без zabbix)
- [x] 2.5 `incident-dashboard-ui`: фильтры — мультивыбор статуса (CSV) и severity, без фильтра источника; вкладка «История» — только журнал инцидента (`IncidentListPage.tsx`, `IncidentDetailPanel.tsx`)

## 3. Согласование с активными изменениями

- [x] 3.1 Убедиться, что `fix-p0-delivery-and-filters` заархивировано и его правки («Список и фильтрация инцидентов», «Конфигурация уведомлений тенанта», форма настроек) вошли в главные спеки
- [x] 3.2 Убедиться, что `enrich-notifications-drop-zabbix` заархивировано (Zabbix удалён, payload обогащён)
- [x] 3.3 Сравнить дельты этого изменения с актуальными текстами главных спек (diff требований «Список и фильтрация инцидентов» и «Срабатывание эскалации по таймауту» — не потерять правки P0/P1)

## 4. Архивация и сопутствующая правка

- [x] 4.1 При sync обновить Purpose-секции затронутых спек (убрать упоминание Zabbix из `alert-ingestion`, «эскалации» из описания виджета в `oncall-schedule-ui`)
- [ ] 4.2 `openspec validate` всех главных спек после архивации
- [x] 4.3 Зафиксировать кандидатов в backlog (из design.md Open Questions): фильтр по источнику, объединённый таймлайн с эскалациями, агрегированный виджет дежурных, `PUT /policies/{id}`, пересчёт severity — например, issue-список или раздел в `docs/`
