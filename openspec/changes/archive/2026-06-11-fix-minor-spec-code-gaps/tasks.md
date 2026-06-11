## 1. (9) Incident: 422-валидация source в grouping-rules

- [x] 1.1 В `services/incident/internal/handler/handler.go` добавить валидацию `{source}` ∈ `alertmanager | grafana` в `PutGroupingRule` и `DeleteGroupingRule`; при недопустимом значении — HTTP 422 с перечнем поддерживаемых источников (по образцу `CreateWebhookToken` в scheduling)
- [x] 1.2 Дополнить интеграционный тест incident: PUT/DELETE с `source: zabbix` возвращают 422, правило не создаётся/не удаляется

## 2. (12) Notification: логирование сбоя запроса конфигурации

- [x] 2.1 В `services/notification/internal/notifier/notifier.go` (`NotifyTriggered`) заменить `cfg, _ := n.cache.Get(...)` на обработку ошибки: `error`-лог с `tenant_slug`, `incident_id` и причиной; доставка продолжается с `cfg == nil` (текущее поведение фоллбэков сохраняется)
- [x] 2.2 Дополнить unit-тест notifier: при ошибке кеша конфигурации email-доставка продолжается, ошибка логируется

## 3. (11, 13) Frontend: хоткей и мобильный список смен

- [x] 3.1 В `frontend/src/pages/IncidentListPage.tsx` удалить пустую привязку `'/': () => {}` из `useKeyMap` (перестать перехватывать `/` и блокировать браузерный quick find)
- [x] 3.2 В `frontend/src/pages/SchedulesPage.tsx` строить мобильный список «Ближайшие 7 дней» по отдельному окну запросов `/shifts?from=<сегодня>&to=<сегодня+7д>` per schedule (не из окна текущего месяца Gantt)
- [x] 3.3 Проверить на границе месяца (например, окно 28-е → +7 дней): смены следующего месяца присутствуют в мобильном списке; прогнать `tsc`/`eslint`

## 4. (8, 10, 14) Выравнивание спецификаций

- [x] 4.1 Delta-спеки этого изменения покрывают пункты 8 (контракт `/state`), 10 (колонка «Alertname»), 14 (формулировка «до 3 попыток»); проверить, что код им соответствует без правок: JSON-поле `escalate_at` в `/state`, колонка «Alertname» в таблице, 3 суммарные попытки в `pkg/amqp`, email- и mattermost-диспетчерах
- [x] 4.2 Синхронизировать/архивировать это изменение строго после `fix-substantial-spec-code-gaps` (delta-спеки `incident-management` и `incident-dashboard-ui` включают его текст)

## 5. Верификация и закрытие

- [x] 5.1 `go build ./... && go test ./...` для incident и notification (включая интеграционные тесты по их инструкции запуска)
- [x] 5.2 Smoke-проверка UI: `/` не перехватывается на странице инцидентов; мобильный список смен корректен на границе месяца
- [x] 5.3 Обновить `docs/spec-vs-code-audit.md`: пометить пункты 8–14 как исправленные этим изменением
