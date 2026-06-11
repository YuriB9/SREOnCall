## 1. Incident-сервис: заполнение tenant_slug

- [ ] 1.1 В `services/incident/internal/consumer/consumer.go` (`handleFiring`) заполнять `TenantSlug: tenantID` при создании инцидента; зафиксировать комментарием, что в событийном конвейере `tenant_id` является slug'ом тенанта (Redis-индекс токенов хранит slug)
- [ ] 1.2 Добавить миграцию `services/incident/migrations/000002_backfill_tenant_slug.up.sql`: `UPDATE incident.incidents SET tenant_slug = tenant_id WHERE tenant_slug = ''` (+ пустой `.down.sql`)
- [ ] 1.3 Дополнить интеграционный тест consumer'а (`consumer_integration_test.go`): событие `incident.created` для авто-созданного инцидента содержит непустой `tenant_slug`, равный `tenant_id` алерта; событие `incident.updated` при авто-резолве — тоже

## 2. Escalation-сервис: fallback на tenant_id

- [ ] 2.1 В `services/escalation/internal/consumer/consumer.go` (оба пути: `handle` и `ProcessDelivery`) перед вызовом `AutoAssign` подставлять `payload.TenantID` вместо пустого `payload.TenantSlug` — симметрично fallback'у в `handler.AttachPolicy`
- [ ] 2.2 Добавить миграцию `services/escalation/migrations/000003_backfill_tenant_slug.up.sql`: `UPDATE escalation.incident_escalation_states SET tenant_slug = tenant_id WHERE tenant_slug = ''` (+ пустой `.down.sql`)
- [ ] 2.3 Дополнить интеграционный тест escalation: при `incident.created` с пустым `tenant_slug` состояние эскалации сохраняется со slug'ом из `tenant_id`, и `escalation.triggered` публикуется с непустым `tenant_slug`

## 3. Проверка цепочки

- [ ] 3.1 Прогнать `go test ./...` для incident и escalation (включая интеграционные тесты по их инструкции запуска)
- [ ] 3.2 Сквозная проверка: алерт через вебхук → инцидент → авто-назначение политики по умолчанию → `escalation.triggered` содержит `tenant_slug` и `oncall_user_id` (вручную через docker-compose/k8s-окружение или e2e, если инфраструктура доступна)
- [ ] 3.3 Обновить `docs/spec-vs-code-audit.md`: пометить пункт 1 как исправленный этим изменением
