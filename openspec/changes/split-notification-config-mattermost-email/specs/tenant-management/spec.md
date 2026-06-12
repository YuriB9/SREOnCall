## MODIFIED Requirements

### Requirement: Конфигурация уведомлений тенанта

Сервис scheduling ДОЛЖЕН (SHALL) хранить per-tenant конфигурацию каналов уведомлений, используемую сервисом notification при отправке.

Конфигурация ДОЛЖНА включать: `mattermost_webhook_url`, `mattermost_channel`, `smtp_from` (опционально, переопределяет глобальный), `email_enabled` (булево, по умолчанию `true`), `email_reply_to` (опционально, адрес Reply-To), `email_subject_prefix` (опционально, префикс темы письма). Поля `email_enabled`, `email_reply_to`, `email_subject_prefix` НЕ являются секретами и НЕ маскируются.

Конфигурация ДОЛЖНА быть доступна по HTTP **исключительно** через `GET/PUT /api/schedules/v1/tenants/{slug}/notification-config`. Альтернативные маршруты к этому ресурсу (в частности, `GET/PUT /api/schedules/v1/{tenant}/notification-config`) НЕ ДОЛЖНЫ существовать: любой обходной путь, возвращающий немаскированный `mattermost_webhook_url` по пользовательскому JWT или перезаписывающий сохранённый URL пустым значением, является нарушением данного требования.

Маскирование `mattermost_webhook_url` (отображение только схемы и хоста) ДОЛЖНО применяться только к запросам, аутентифицированным пользовательским JWT. Запросы, аутентифицированные сервисным ключом (`X-Admin-Key`), ДОЛЖНЫ получать полный URL. При неопределённом способе аутентификации URL ДОЛЖЕН маскироваться (безопасное умолчание).

`GET` по каноническому маршруту для тенанта, у которого конфигурация ещё ни разу не сохранялась (строка отсутствует), ДОЛЖЕН возвращать HTTP **200 с дефолтным конфигом** (`email_enabled=true`, остальные поля пустыми строками), а НЕ 404. Это гарантирует, что сервис notification всегда получает корректно определённый конфиг и не интерпретирует отсутствие настроек как сбой.

PUT конфигурации с пустым или отсутствующим `mattermost_webhook_url` ДОЛЖЕН сохранять текущее сохранённое значение URL без изменений; остальные переданные поля обновляются. Перезапись URL происходит только при передаче непустого значения. Поля, отсутствующие в теле PUT, ДОЛЖНЫ сохранять текущее значение (частичное обновление), что позволяет сохранять секцию Mattermost и секцию Email независимо.

#### Scenario: Установка конфигурации

- **WHEN** администратор выполняет PUT на `/api/schedules/v1/tenants/{slug}/notification-config` с валидными полями
- **THEN** конфигурация сохраняется и становится активной для всех уведомлений тенанта

#### Scenario: Получение конфигурации

- **WHEN** выполняется GET на `/api/schedules/v1/tenants/{slug}/notification-config` пользователем (JWT)
- **THEN** возвращается текущая конфигурация; `mattermost_webhook_url` маскируется (отображается только домен); поля `email_enabled`, `email_reply_to`, `email_subject_prefix` возвращаются без маскирования

#### Scenario: Получение конфигурации сервисом

- **WHEN** выполняется GET на `/api/schedules/v1/tenants/{slug}/notification-config` с валидным заголовком `X-Admin-Key`
- **THEN** возвращается конфигурация с полным немаскированным `mattermost_webhook_url` и полями email-канала

#### Scenario: Получение конфигурации для незаполненного тенанта

- **WHEN** выполняется GET на канонический `/api/schedules/v1/tenants/{slug}/notification-config` для тенанта, у которого строка конфигурации ещё не создана
- **THEN** сервис возвращает HTTP 200 с дефолтным конфигом (`email_enabled=true`, `mattermost_webhook_url`/`mattermost_channel`/`smtp_from`/`email_reply_to`/`email_subject_prefix` — пустые), а не 404

#### Scenario: Обходные маршруты отсутствуют

- **WHEN** выполняется GET или PUT на `/api/schedules/v1/{tenant}/notification-config` (tenant-scoped путь вне канонического `/tenants/{slug}/...`)
- **THEN** сервис возвращает HTTP 404/405; немаскированный `mattermost_webhook_url` недоступен ни по одному маршруту без сервисной аутентификации

#### Scenario: PUT с пустым URL не затирает сохранённый вебхук

- **WHEN** администратор выполняет PUT с `mattermost_channel: "#new"` и пустым/отсутствующим `mattermost_webhook_url`, при том что для тенанта уже сохранён непустой URL
- **THEN** `mattermost_channel` обновляется, а сохранённый `mattermost_webhook_url` остаётся прежним

#### Scenario: PUT с непустым URL обновляет вебхук

- **WHEN** администратор выполняет PUT с непустым валидным `mattermost_webhook_url`
- **THEN** сохранённый URL заменяется переданным значением

#### Scenario: Независимое сохранение секции Email не затирает Mattermost

- **WHEN** выполняется PUT только с полями секции Email (`email_enabled`, `smtp_from`, `email_reply_to`, `email_subject_prefix`) без полей Mattermost, при сохранённых ранее `mattermost_webhook_url` и `mattermost_channel`
- **THEN** email-поля обновляются, а `mattermost_webhook_url` и `mattermost_channel` остаются прежними
