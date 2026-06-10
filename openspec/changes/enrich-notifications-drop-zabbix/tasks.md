# Задачи: Обогащение уведомлений и вывод Zabbix из поддержки

## 1. Обогащение событий эскалации данными инцидента

- [ ] 1.1 Миграция БД escalation: nullable-колонки `incident_title`, `incident_severity`, `incident_status` в `escalation_states`
- [ ] 1.2 `services/escalation/internal/consumer/consumer.go`: расширить `incidentPayload` полями `title`, `severity`; передавать их в `AutoAssign`
- [ ] 1.3 `services/escalation/internal/escalator/escalator.go`: сохранять данные инцидента в состоянии при `AssignPolicy`; включать их в `TriggeredEvent` в `triggerTier`
- [ ] 1.4 `services/escalation/internal/publisher`: поля `incident_title`, `incident_severity`, `incident_status` в `TriggeredEvent`
- [ ] 1.5 Ручная привязка (`handler.AttachPolicy`): запрос `GET /api/incidents/v1/{tenant}/incidents/{id}` с `X-Admin-Key` (клиент incident-сервиса); при сбое — пустые поля + warn-лог
- [ ] 1.6 Тесты escalator: данные из события попадают в triggered-событие; ручная привязка при недоступном incident не блокируется

## 2. Содержимое уведомлений

- [ ] 2.1 `services/notification/internal/notifier/notifier.go`: расширить `TriggeredEvent` новыми полями; убрать подмену Title строкой «Escalation tier N» (оставить как fallback при пустом title)
- [ ] 2.2 `services/notification/internal/config`: параметр `FRONTEND_BASE_URL`; warn-лог при старте, если не задан
- [ ] 2.3 `services/notification/internal/dispatcher/email.go`: тема `[SRE OnCall] [{severity}] {title}`, тело с ID, заголовком, severity, статусом, tier, ссылкой `{base}/{tenant_slug}/incidents?incident={id}` и временной меткой; без ссылки при пустом base URL
- [ ] 2.4 `services/notification/internal/dispatcher/mattermost.go` + `notifier.go`: сообщение с упоминанием, ID, заголовком, severity, статусом, tier и ссылкой
- [ ] 2.5 Тесты notifier/dispatcher: полное содержимое при обогащённом payload; резервное содержимое при событии без полей инцидента; отсутствие ссылки без `FRONTEND_BASE_URL`

## 3. Удаление Zabbix

- [ ] 3.1 `services/scheduling/internal/handler/handler.go:596`: `validSources` → `alertmanager | grafana`; обновить текст ошибки 422
- [ ] 3.2 `services/incident/internal/store/store.go:495` и `internal/domain/incident.go`: источники и умолчания группировки без zabbix
- [ ] 3.3 `pkg/domain/alert.go`: удалить константу `SourceZabbix`; проверить компиляцию всех сервисов
- [ ] 3.4 `frontend/src/pages/TenantSettingsPage.tsx:19`: `VALID_SOURCES` без zabbix
- [ ] 3.5 Обновить упоминание Zabbix в Purpose главной спеки `alert-ingestion` при синхронизации дельт (sync/archive)
- [ ] 3.6 Release notes: рекомендация отозвать существующие webhook-токены с `source: zabbix`

## 4. Чистка фронтенда

- [ ] 4.1 Удалить неиспользуемые хуки `useOnCallNow` и `useScheduleWindow` из `frontend/src/api/schedules.ts` (второй обращается к `/oncall` с игнорируемыми параметрами `from`/`to`)
- [ ] 4.2 Прогнать `tsc -b` — убедиться в отсутствии оставшихся ссылок

## 5. Деплой и проверка

- [ ] 5.1 `deploy/k8s/notification/configmap.yaml`: добавить `FRONTEND_BASE_URL`
- [ ] 5.2 `go build ./...`, `go test ./...` по затронутым сервисам
- [ ] 5.3 Сквозная проверка: алерт → инцидент → tier-1 эскалация → email и Mattermost содержат заголовок, severity, статус и рабочую ссылку на инцидент
- [ ] 5.4 Проверка: POST webhook-токена с `source: zabbix` возвращает 422; диалог создания токена в UI предлагает только два источника
