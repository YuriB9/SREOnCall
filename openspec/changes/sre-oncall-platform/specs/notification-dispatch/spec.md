# ADDED Requirements

## Requirement: Потребление событий эскалации

Сервис notification ДОЛЖЕН потреблять сообщения `escalation.triggered` и `escalation.exhausted` из очереди `escalations.notification` в RabbitMQ. Payload события содержит `oncall_user_id` и `oncall_username`, разрешённые escalation service в момент публикации — notification не делает синхронных вызовов в scheduling за on-call данными.

Первичное оповещение при создании инцидента покрывается событием `escalation.triggered` для tier 1 (публикуется escalation service сразу после авто-назначения политики).

### Scenario: Уведомление при срабатывании эскалации

- **WHEN** потреблено событие `escalation.triggered` с `oncall_user_id` в payload
- **THEN** сервис отправляет уведомление пользователю через все его настроенные каналы (`user_contacts` по `(oncall_user_id, tenant_id)`)

### Scenario: Уведомление при исчерпании эскалации

- **WHEN** потреблено событие `escalation.exhausted`
- **THEN** уведомление отправляется на Mattermost-канал тенанта без @mention конкретного пользователя

## Requirement: Отправка Email-уведомлений через SMTP

Сервис notification ДОЛЖЕН отправлять уведомления об инцидентах и эскалациях по электронной почте через настроенный SMTP-сервер.

Email ДОЛЖЕН содержать: ID инцидента, заголовок, severity, текущий статус, ссылку на эндпоинт API инцидента и временную метку.

### Scenario: Успешная отправка email

- **WHEN** для пользователя сработало уведомление и для него настроен адрес электронной почты
- **THEN** письмо отправляется через SMTP с корректной темой и содержимым

### Scenario: SMTP-сервер недоступен

- **WHEN** SMTP-сервер недостижим
- **THEN** сервис выполняет до 3 повторных попыток с экспоненциальной задержкой, затем помечает доставку как `failed` и логирует структурированную ошибку

## Requirement: Отправка уведомлений в Mattermost через входящий вебхук

Сервис notification ДОЛЖЕН отправлять уведомления на настроенный URL входящего вебхука Mattermost.

Сообщение отправляется в канал тенанта (`mattermost_channel` из `tenant_notification_config`) и ДОЛЖНО включать: ID инцидента, severity, заголовок, статус и упоминание (@) `mattermost_username` дежурного из `user_contacts`, если настроено.

### Scenario: Успешная отправка в Mattermost

- **WHEN** для уведомления настроен URL вебхука Mattermost
- **THEN** сервис отправляет POST с отформатированным сообщением на URL вебхука

### Scenario: Вебхук Mattermost возвращает не-2xx

- **WHEN** URL вебхука Mattermost возвращает ответ с ошибкой
- **THEN** сервис выполняет до 3 повторных попыток, затем помечает доставку как `failed`

## Requirement: Rate-limiting уведомлений на контакт

Сервис notification ДОЛЖЕН применять настраиваемый rate-limit на контакт (по умолчанию: не более 5 уведомлений за 10 минут на контакт) через Redis token bucket.

### Scenario: Лимит не превышен

- **WHEN** уведомление отправляется и контакт не достиг лимита
- **THEN** уведомление отправляется и токен-бакет уменьшается

### Scenario: Лимит превышен

- **WHEN** уведомление превысило бы per-contact rate-limit
- **THEN** уведомление отбрасывается; в `notification_log` записывается запись со статусом `rate_limited`; в структурированный лог выводится предупреждение с `user_id`, `tenant_id` и каналом

## Requirement: Журнал доставки уведомлений

Сервис notification ДОЛЖЕН записывать каждую попытку отправки (канал, получатель, статус, временная метка, сообщение об ошибке при неудаче) в PostgreSQL. Допустимые статусы: `delivered`, `failed`, `rate_limited`.

### Scenario: Запись успешной доставки

- **WHEN** уведомление успешно доставлено
- **THEN** в журнал записывается запись со статусом `delivered` и временной меткой

### Scenario: Запись неудачной доставки

- **WHEN** все повторные попытки доставки исчерпаны
- **THEN** в журнал записывается запись со статусом `failed` и деталями ошибки

## Requirement: Настройка контактных данных пользователя

Сервис notification ДОЛЖЕН хранить per-user конфигурацию контактов: `email`, `mattermost_username` и `enabled_channels` (список). Конфигурация привязана к паре `(user_id, tenant_id)`.

### Scenario: Создание или обновление контактной конфигурации

- **WHEN** выполняется PUT на `/api/notifications/v1/{tenant}/contacts/{userId}` с валидными полями
- **THEN** конфигурация сохраняется для данного пользователя в данном тенанте

### Scenario: Уведомление отправляется только по включённым каналам

- **WHEN** у пользователя `enabled_channels: ["email"]` для данного тенанта
- **THEN** используется только email; Mattermost пропускается даже при наличии tenant_notification_config

## Requirement: Использование per-tenant конфигурации Mattermost

Сервис notification ДОЛЖЕН брать `mattermost_webhook_url` и `mattermost_channel` из `tenant_notification_config` тенанта, к которому относится инцидент, а не из глобальной конфигурации.

### Scenario: Отправка в Mattermost с per-tenant webhook

- **WHEN** формируется уведомление для инцидента тенанта A
- **THEN** используется `mattermost_webhook_url` из `tenant_notification_config` тенанта A

### Scenario: Mattermost не настроен для тенанта

- **WHEN** у тенанта не задан `mattermost_webhook_url`
- **THEN** Mattermost-уведомление не отправляется; в журнал записывается предупреждение
