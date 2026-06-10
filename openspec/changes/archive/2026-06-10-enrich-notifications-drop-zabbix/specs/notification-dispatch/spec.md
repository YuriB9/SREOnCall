## MODIFIED Requirements

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

### Requirement: Отправка уведомлений в Mattermost через входящий вебхук

Сервис notification ДОЛЖЕН (SHALL) отправлять уведомления на настроенный URL входящего вебхука Mattermost.

Сообщение отправляется в канал тенанта (`mattermost_channel` из `tenant_notification_config`) и ДОЛЖНО включать: ID инцидента, severity, заголовок инцидента, статус, уровень эскалации (tier), ссылку на инцидент в веб-интерфейсе (если задан `FRONTEND_BASE_URL`) и упоминание (@) `mattermost_username` дежурного из `user_contacts`, если настроено.

#### Scenario: Успешная отправка в Mattermost

- **WHEN** для уведомления настроен URL вебхука Mattermost
- **THEN** сервис отправляет POST с отформатированным сообщением (упоминание, ID, заголовок, severity, статус, tier, ссылка) на URL вебхука

#### Scenario: Вебхук Mattermost возвращает не-2xx

- **WHEN** URL вебхука Mattermost возвращает ответ с ошибкой
- **THEN** сервис выполняет до 3 повторных попыток, затем помечает доставку как `failed`
