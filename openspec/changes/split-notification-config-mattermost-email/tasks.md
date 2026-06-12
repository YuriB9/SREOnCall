## 1. Бэкенд: scheduling — схема и модель

- [ ] 1.1 Добавить миграцию `services/scheduling/migrations/000002_notification_email_fields.up.sql`: `ALTER TABLE tenant_notification_config ADD COLUMN email_enabled boolean NOT NULL DEFAULT true, ADD COLUMN email_reply_to text NOT NULL DEFAULT '', ADD COLUMN email_subject_prefix text NOT NULL DEFAULT ''`; и `.down.sql` с `DROP COLUMN` для всех трёх
- [ ] 1.2 Расширить `store.NotificationConfig` ([store.go:394](../../../services/scheduling/internal/store/store.go#L394)) полями `EmailEnabled bool`, `EmailReplyTo string`, `EmailSubjectPrefix string` с json-тегами `email_enabled`/`email_reply_to`/`email_subject_prefix`
- [ ] 1.3 Обновить `GetNotificationConfig` (SELECT + Scan новых колонок) и `UpsertNotificationConfig` (INSERT/UPDATE новых колонок)

## 2. Бэкенд: scheduling — хендлеры GET/PUT

- [ ] 2.1 В `GetTenantNotificationConfig` ([handler.go:635](../../../services/scheduling/internal/handler/handler.go#L635)) добавить в JSON-ответ `email_enabled`, `email_reply_to`, `email_subject_prefix` (без маскирования)
- [ ] 2.2 В `GetTenantNotificationConfig` при `store.ErrNotFound` возвращать HTTP 200 с дефолтным конфигом (`email_enabled=true`, остальные поля пустые) вместо 404 — только для канонического маршрута `/tenants/{slug}/...`; защита неканонических путей (404/405) не затрагивается
- [ ] 2.3 Переписать `PutTenantNotificationConfig` на частичное обновление: читать тело в структуру с указателями (`*string`/`*bool`), загружать текущую запись и обновлять только присутствующие поля; сохранить особый случай `mattermost_webhook_url` (пустое = «не менять»)
- [ ] 2.4 Покрыть GET/PUT в `handler_integration_test.go`: GET для незаполненного тенанта → 200+дефолты (не 404); раздельное сохранение секций не затирает чужие поля; `email_enabled=false` сохраняется; дефолты для старых записей

## 3. Бэкенд: notification — приём и применение полей

- [ ] 3.1 Расширить `schedclient.Config` ([client.go](../../../services/notification/internal/schedclient/client.go)) полями `EmailEnabled`, `EmailReplyTo`, `EmailSubjectPrefix`
- [ ] 3.2 Добавить в `dispatcher.EmailMessage` поля `ReplyTo` и `SubjectPrefix`; в `Email.Send` ([email.go](../../../services/notification/internal/dispatcher/email.go)) добавлять префикс в начало темы и заголовок `Reply-To` (только если непустой), включая fallback-тему
- [ ] 3.3 В `notifier` ветке `ChannelEmail` ([notifier.go:191](../../../services/notification/internal/notifier/notifier.go#L191)): при `cfg == nil` или отсутствии конфига считать email включённым (дефолт), пропускать email только при явном `cfg.EmailEnabled == false` (лог info, без записи `failed`); заполнять `ReplyTo`/`SubjectPrefix` из cfg
- [ ] 3.4 Обновить/добавить юнит-тесты `notifier_test.go` и `email`: незаполненный/`nil` конфиг → email уходит через глобальный SMTP; явный `email_enabled=false` → email пропускается; Reply-To и префикс попадают в письмо
- [ ] 3.5 (Опционально) `schedclient_test.go`: GET для незаполненного тенанта теперь 200 → возвращается дефолтный конфиг (не `nil`); ветка 404→nil остаётся как защитная

## 4. Фронтенд: типы и API

- [ ] 4.1 Расширить интерфейс `NotificationConfig` ([types.ts:157](../../../frontend/src/api/types.ts#L157)) полями `email_enabled: boolean`, `email_reply_to: string`, `email_subject_prefix: string`

## 5. Фронтенд: разделение формы на две секции

- [ ] 5.1 В [NotificationConfigPage.tsx](../../../frontend/src/pages/settings/NotificationConfigPage.tsx) выделить две секции/компонента: `MattermostSection` (`mattermost_webhook_url`, `mattermost_channel`) и `EmailSection` (`email_enabled`, `smtp_from`, `email_reply_to`, `email_subject_prefix`), каждая со своей кнопкой «Сохранить» и своим частичным PUT
- [ ] 5.2 Сохранить логику маскированного `mattermost_webhook_url` (не предзаполнять, не слать пустым) в `MattermostSection`
- [ ] 5.3 В `EmailSection`: переключатель `email_enabled`, валидация `smtp_from` и `email_reply_to` через `isValidEmail`, мягкое ограничение длины `email_subject_prefix` (64 симв.); предзаполнять поля из GET
- [ ] 5.4 Toast об успехе/ошибке для каждой секции независимо

## 6. Спеки и верификация

- [ ] 6.1 Прогнать `gofmt`/`go build ./...` и `go test ./...` в затронутых сервисах; `npm run build`/линт во фронте
- [ ] 6.2 Проверить экран `/[tenant]/settings/notifications` вручную: две секции, раздельное сохранение, новые поля сохраняются и перечитываются
- [ ] 6.3 Синхронизировать главные спеки (`/opsx:sync` или `openspec-sync-specs`) после реализации
