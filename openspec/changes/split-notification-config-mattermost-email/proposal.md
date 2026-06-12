## Why

Форма «Конфигурация уведомлений» (`/[tenant]/settings/notifications`) сейчас смешивает поля Mattermost и Email в одном плоском списке, из-за чего администратору неочевидно, какие настройки к какому каналу относятся. Кроме того, Email-канал настраивается беднее остальных: на уровне тенанта есть только адрес отправителя (`smtp_from`), тогда как темы писем и адрес для ответа фиксированы глобально.

## What Changes

- Экран конфигурации уведомлений разбивается на **две независимые логические секции** — «Mattermost» и «Email» — каждая со своими полями и собственной кнопкой «Сохранить».
- В секцию Email добавляются новые per-tenant настройки (без секретов):
  - `email_enabled` — переключатель, позволяющий полностью отключить email-канал для тенанта;
  - `email_reply_to` — адрес для ответа (Reply-To);
  - `email_subject_prefix` — префикс темы письма (например, `[ACME PROD]`).
- Бэкенд scheduling: в таблицу `tenant_notification_config` добавляются колонки `email_enabled`, `email_reply_to`, `email_subject_prefix`; они отдаются в `GET` и принимаются в `PUT /api/schedules/v1/tenants/{slug}/notification-config`.
- Сервис notification использует новые поля при отправке email: уважает `email_enabled` (выключенный канал пропускается), подставляет `Reply-To` и префикс темы из per-tenant конфигурации.
- Семантика частичного обновления Mattermost-вебхука (пустой `mattermost_webhook_url` = «не менять») сохраняется без изменений.
- **Исправление: эскалации при незаполненном конфиге.** Сейчас `GET notification-config` для тенанта без сохранённой строки возвращает 404; на стороне notification это превращается в `nil`, который кэшируется на весь TTL (5 минут), из-за чего после первого сохранения настроек уведомления продолжают «молчать». `GET` ДОЛЖЕН возвращать **200 с дефолтным конфигом** (`email_enabled=true`, остальные поля пустые), когда строки ещё нет. Отсутствующий/`nil` конфиг в notifier трактуется как «email включён» (email уходит через глобальный `SMTP_FROM`), Mattermost корректно пропускается до настройки webhook.

## Capabilities

### New Capabilities
<!-- нет новых capability -->

### Modified Capabilities
- `tenant-settings-ui`: требование «Форма конфигурации уведомлений» переписывается под две раздельные секции (Mattermost / Email) и новые поля Email (`email_enabled`, `email_reply_to`, `email_subject_prefix`).
- `notification-dispatch`: требования «Отправка Email-уведомлений через SMTP» и «Авторизованное получение per-tenant конфигурации из scheduling» расширяются учётом `email_enabled`, `email_reply_to`, `email_subject_prefix` и устойчивостью к незаполненному конфигу (дефолт «email включён»).
- `tenant-management`: требование «Конфигурация уведомлений тенанта» расширяется тремя новыми полями и меняет поведение `GET` для несуществующей строки — 200 с дефолтным конфигом вместо 404.

## Impact

- **Фронтенд**: `frontend/src/pages/settings/NotificationConfigPage.tsx` (разделение на две секции + новые поля), `frontend/src/api/types.ts` (`NotificationConfig`), при необходимости `frontend/src/api/tenantSettings.ts`.
- **services/scheduling**: миграция `tenant_notification_config` (+3 колонки), `internal/store/store.go` (`NotificationConfig`, GET/Upsert), `internal/handler/handler.go` (`GetTenantNotificationConfig` — 200 с дефолтами вместо 404; `PutTenantNotificationConfig` — частичное обновление новых полей).
- **services/notification**: `internal/schedclient/client.go` (новые поля в DTO), `internal/notifier/notifier.go` (дефолт «email включён» при `cfg == nil`, учёт `email_enabled`) и `internal/dispatcher/email.go` (`Reply-To`, префикс темы).
- **API**: расширяется тело `GET`/`PUT notification-config` тремя необязательными полями. Не **BREAKING** — старые клиенты и старые записи БД продолжают работать (значения по умолчанию: `email_enabled=true`, пустые строки).
- **События RabbitMQ**: не затрагиваются.
- **БД**: одна миграция в схеме scheduling; обратная совместимость через `DEFAULT`.
