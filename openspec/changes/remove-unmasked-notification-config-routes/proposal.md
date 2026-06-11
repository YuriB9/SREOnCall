## Why

В scheduling-сервисе помимо канонических эндпоинтов `/api/schedules/v1/tenants/{slug}/notification-config` существует параллельная, не описанная в спецификации пара `GET/PUT /api/schedules/v1/{tenant}/notification-config`, которая обходит обе защиты канонического контракта: GET отдаёт **немаскированный** `mattermost_webhook_url` любому участнику тенанта (не только администратору и не только сервисному ключу), а PUT с пустым полем **затирает** сохранённый вебхук. Это нарушает требование маскирования из спеки `tenant-management`. Фронтенд и notification-сервис этими маршрутами не пользуются; единственный потребитель — один интеграционный тест scheduling. Обнаружено при сверке спецификаций с кодом (`docs/spec-vs-code-audit.md`, пункт 2).

## What Changes

- **BREAKING (формально):** удаляются маршруты `GET /api/schedules/v1/{tenant}/notification-config` и `PUT /api/schedules/v1/{tenant}/notification-config` вместе с обработчиками `GetNotificationConfig`/`PutNotificationConfig`. Фактических потребителей вне репозитория нет; внутри репозитория — только интеграционный тест.
- Интеграционный тест scheduling переводится на канонические маршруты `/api/schedules/v1/tenants/{slug}/notification-config` и дополняется проверками маскирования (JWT — маска, `X-Admin-Key` — полный URL) и сохранения вебхука при PUT с пустым полем.
- Спецификация `tenant-management` фиксирует, что конфигурация уведомлений доступна **только** через `/api/schedules/v1/tenants/{slug}/notification-config` и альтернативных маршрутов, раскрывающих немаскированный URL пользовательским JWT, не существует.

## Capabilities

### New Capabilities

Нет — новые возможности не вводятся.

### Modified Capabilities

- `tenant-management`: требование «Конфигурация уведомлений тенанта» уточняется — единственный HTTP-интерфейс конфигурации это `/api/schedules/v1/tenants/{slug}/notification-config`; запросы немаскированного URL возможны только с сервисной аутентификацией, обходных маршрутов нет.

## Impact

- `services/scheduling/cmd/server/main.go` — удаление двух маршрутов из tenant-scoped группы.
- `services/scheduling/internal/handler/handler.go` — удаление обработчиков `GetNotificationConfig` и `PutNotificationConfig` (канонические `GetTenantNotificationConfig`/`PutTenantNotificationConfig` остаются без изменений).
- `services/scheduling/internal/handler/handler_integration_test.go` — перевод теста notification-config на канонические маршруты, расширение проверок.
- Фронтенд, notification-сервис, escalation — без изменений (используют канонические маршруты).
- `docs/spec-vs-code-audit.md` — пометка пункта 2 как исправленного.
