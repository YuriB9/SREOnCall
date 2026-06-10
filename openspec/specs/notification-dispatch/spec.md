# Notification Dispatch

## Purpose

Отправка уведомлений об инцидентах и эскалациях по Email и Mattermost с rate-limiting, журналированием доставки и per-tenant конфигурацией.

## Requirements

### Requirement: Потребление событий эскалации

Сервис notification ДОЛЖЕН (SHALL) потреблять сообщения `escalation.triggered` и `escalation.exhausted` из очереди `escalations.notification` в RabbitMQ. Payload события `escalation.triggered` содержит `oncall_user_id` и `oncall_username`, разрешённые escalation service в момент публикации, а также данные инцидента `incident_title`, `incident_severity`, `incident_status` — notification не делает синхронных вызовов в другие сервисы ни за on-call-данными, ни за содержимым уведомления.

Первичное оповещение при создании инцидента покрывается событием `escalation.triggered` для tier 1 (публикуется escalation service сразу после авто-назначения политики).

При отсутствии полей инцидента в payload (события от старой версии escalation) сервис ДОЛЖЕН отправить уведомление с резервной темой/текстом без этих данных, не прерывая доставку.

#### Scenario: Уведомление при срабатывании эскалации

- **WHEN** потреблено событие `escalation.triggered` с `oncall_user_id` в payload
- **THEN** сервис отправляет уведомление пользователю через все его настроенные каналы (`user_contacts` по `(oncall_user_id, tenant_id)`), используя `incident_title`, `incident_severity` и `incident_status` из payload

#### Scenario: Событие без данных инцидента обрабатывается

- **WHEN** потреблено событие `escalation.triggered` без полей `incident_title`/`incident_severity`/`incident_status`
- **THEN** уведомление отправляется с резервным содержимым (ID инцидента и tier), доставка не прерывается

#### Scenario: Уведомление при исчерпании эскалации

- **WHEN** потреблено событие `escalation.exhausted`
- **THEN** уведомление отправляется в Mattermost-канал тенанта без @mention конкретного пользователя

---

### Requirement: Отправка Email-уведомлений через SMTP

Сервис notification ДОЛЖЕН (SHALL) отправлять уведомления об инцидентах и эскалациях по электронной почте через настроенный SMTP-сервер.

Email ДОЛЖЕН содержать: ID инцидента, заголовок инцидента (`incident_title` из payload события), severity, текущий статус, уровень эскалации (tier), ссылку на инцидент в веб-интерфейсе и временную метку. Ссылка строится из настраиваемого базового URL фронтенда (`FRONTEND_BASE_URL`) в формате `{base}/{tenant_slug}/incidents?incident={incident_id}`; если базовый URL не задан, письмо отправляется без ссылки с warn-записью в лог.

#### Scenario: Успешная отправка email

- **WHEN** для пользователя сработало уведомление и для него настроен адрес электронной почты
- **THEN** письмо отправляется через SMTP; тема содержит severity и заголовок инцидента, тело — ID, заголовок, severity, статус, tier, ссылку на инцидент и временную метку

#### Scenario: Базовый URL фронтенда не настроен

- **WHEN** `FRONTEND_BASE_URL` не задан в конфигурации сервиса
- **THEN** письмо отправляется без ссылки на инцидент, в лог пишется предупреждение

#### Scenario: SMTP-сервер недоступен

- **WHEN** SMTP-сервер недостижим
- **THEN** сервис выполняет до 3 повторных попыток с экспоненциальной задержкой, затем помечает доставку как `failed` и логирует структурированную ошибку

---

### Requirement: Отправка уведомлений в Mattermost через входящий вебхук

Сервис notification ДОЛЖЕН (SHALL) отправлять уведомления на настроенный URL входящего вебхука Mattermost.

Сообщение отправляется в канал тенанта (`mattermost_channel` из `tenant_notification_config`) и ДОЛЖНО включать: ID инцидента, severity, заголовок инцидента, статус, уровень эскалации (tier), ссылку на инцидент в веб-интерфейсе (если задан `FRONTEND_BASE_URL`) и упоминание (@) `mattermost_username` дежурного из `user_contacts`, если настроено.

#### Scenario: Успешная отправка в Mattermost

- **WHEN** для уведомления настроен URL вебхука Mattermost
- **THEN** сервис отправляет POST с отформатированным сообщением (упоминание, ID, заголовок, severity, статус, tier, ссылка) на URL вебхука

#### Scenario: Вебхук Mattermost возвращает не-2xx

- **WHEN** URL вебхука Mattermost возвращает ответ с ошибкой
- **THEN** сервис выполняет до 3 повторных попыток, затем помечает доставку как `failed`

---

### Requirement: Rate-limiting уведомлений на контакт

Сервис notification ДОЛЖЕН (SHALL) применять настраиваемый rate-limit на контакт (по умолчанию: не более 5 уведомлений за 10 минут на контакт) через Redis token bucket.

#### Scenario: Лимит не превышен

- **WHEN** уведомление отправляется и контакт не достиг лимита
- **THEN** уведомление отправляется и токен-бакет уменьшается

#### Scenario: Лимит превышен

- **WHEN** уведомление превысило бы per-contact rate-limit
- **THEN** уведомление отбрасывается; в `notification_log` записывается запись со статусом `rate_limited`; в структурированный лог выводится предупреждение с `user_id`, `tenant_id` и каналом

---

### Requirement: Журнал доставки уведомлений

Сервис notification ДОЛЖЕН (SHALL) записывать каждую попытку отправки (канал, получатель, статус, временная метка, сообщение об ошибке при неудаче) в PostgreSQL. Допустимые статусы: `delivered`, `failed`, `rate_limited`.

#### Scenario: Запись успешной доставки

- **WHEN** уведомление успешно доставлено
- **THEN** в журнал записывается запись со статусом `delivered` и временной меткой

#### Scenario: Запись неудачной доставки

- **WHEN** все повторные попытки доставки исчерпаны
- **THEN** в журнал записывается запись со статусом `failed` и деталями ошибки

---

### Requirement: Настройка контактных данных пользователя

Сервис notification ДОЛЖЕН (SHALL) хранить per-user конфигурацию контактов: `email`, `mattermost_username` и `enabled_channels` (список). Конфигурация привязана к паре `(user_id, tenant_id)`.

#### Scenario: Создание или обновление контактной конфигурации

- **WHEN** выполняется PUT на `/api/notifications/v1/{tenant}/contacts/{userId}` с валидными полями
- **THEN** конфигурация сохраняется для данного пользователя в данном тенанте

#### Scenario: Уведомление отправляется только по включённым каналам

- **WHEN** у пользователя `enabled_channels: ["email"]` для данного тенанта
- **THEN** используется только email; Mattermost пропускается даже при наличии tenant_notification_config

---

### Requirement: Использование per-tenant конфигурации Mattermost

Сервис notification ДОЛЖЕН (SHALL) брать `mattermost_webhook_url` и `mattermost_channel` из `tenant_notification_config` тенанта, к которому относится инцидент, а не из глобальной конфигурации.

#### Scenario: Отправка в Mattermost с per-tenant webhook

- **WHEN** формируется уведомление для инцидента тенанта A
- **THEN** используется `mattermost_webhook_url` из `tenant_notification_config` тенанта A

#### Scenario: Mattermost не настроен для тенанта

- **WHEN** у тенанта не задан `mattermost_webhook_url`
- **THEN** Mattermost-уведомление не отправляется; в журнал записывается предупреждение

---

### Requirement: Авторизованное получение per-tenant конфигурации из scheduling

Сервис notification ДОЛЖЕН (SHALL) запрашивать конфигурацию уведомлений тенанта (`GET /api/schedules/v1/tenants/{slug}/notification-config`) с сервисной аутентификацией — заголовком `X-Admin-Key`, значение которого поступает из конфигурации сервиса (k8s Secret).

Полученная конфигурация ДОЛЖНА содержать полный (немаскированный) `mattermost_webhook_url`, пригодный для отправки сообщений. Сервис НЕ ДОЛЖЕН использовать маскированное значение (`scheme://host` без пути) для отправки: если в ответе получен маскированный или пустой URL, Mattermost-доставка пропускается с записью `failed` в журнал доставки и структурированной ошибкой в лог.

#### Scenario: Запрос конфигурации с сервисным ключом

- **WHEN** сервис notification запрашивает конфигурацию тенанта у scheduling
- **THEN** запрос содержит заголовок `X-Admin-Key`, scheduling возвращает HTTP 200 с полным `mattermost_webhook_url`, и notification использует его для отправки в Mattermost

#### Scenario: Сбой авторизации при получении конфигурации

- **WHEN** scheduling отвечает 401/403 на запрос конфигурации (ключ отсутствует или неверен)
- **THEN** сервис notification логирует структурированную ошибку с уровнем `error` и тенантом; Mattermost-доставка для события пропускается с записью `failed` в журнал, email-доставка продолжается с глобальным `smtp_from`
