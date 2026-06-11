## 1. Удаление параллельных маршрутов и обработчиков

- [x] 1.1 Удалить маршруты `r.Get("/notification-config", ...)` и `r.Put("/notification-config", ...)` из tenant-scoped группы `/api/schedules/v1/{tenant}` в `services/scheduling/cmd/server/main.go`
- [x] 1.2 Удалить обработчики `GetNotificationConfig` и `PutNotificationConfig` из `services/scheduling/internal/handler/handler.go` (канонические `GetTenantNotificationConfig`/`PutTenantNotificationConfig` не трогать)
- [x] 1.3 Убедиться grep'ом, что в репозитории не осталось ссылок на удалённые обработчики и на путь `/{tenant}/notification-config` (кроме архивов openspec и аудита)

## 2. Перевод и расширение интеграционного теста

- [x] 2.1 Перевести тест notification-config в `services/scheduling/internal/handler/handler_integration_test.go` с `/api/schedules/v1/{tenant}/notification-config` на канонические `/api/schedules/v1/tenants/{slug}/notification-config` (роутинг тестового сервера и сами запросы)
- [x] 2.2 Добавить проверку: GET с пользовательским JWT (или без сервисного ключа) возвращает маскированный `mattermost_webhook_url` (`scheme://host/***`)
- [x] 2.3 Добавить проверку: GET с заголовком `X-Admin-Key` возвращает полный немаскированный URL
- [x] 2.4 Добавить проверку: PUT с пустым/отсутствующим `mattermost_webhook_url` обновляет остальные поля и не затирает сохранённый URL

## 3. Верификация

- [x] 3.1 `go build ./...` и `go test ./...` в `services/scheduling` (включая интеграционные тесты по их инструкции запуска)
- [x] 3.2 Smoke-проверка: фронтенд (страница настроек тенанта) и notification-сервис продолжают работать — оба используют канонические маршруты, изменений не требуют
