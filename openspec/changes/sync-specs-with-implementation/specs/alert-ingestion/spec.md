## MODIFIED Requirements

### Requirement: Идентификация тенанта по вебхук-токену

Все webhook-эндпоинты ingestion-сервиса ДОЛЖНЫ (SHALL) требовать заголовок `X-Webhook-Token`. Сервис ДОЛЖЕН вычислять SHA-256 хэш токена и выполнять lookup в Redis-индексе `oncall:tokens:{hash}` (поддерживается сервисом scheduling при создании/отзыве токенов), извлекая `tenant_id` для дальнейшей обработки алерта. Прямое чтение таблиц scheduling-сервиса НЕ ДОЛЖНО использоваться.

#### Scenario: Валидный токен

- **WHEN** запрос содержит `X-Webhook-Token`, хэш которого присутствует в Redis-индексе `oncall:tokens:{hash}`
- **THEN** `tenant_id` из индекса присваивается алерту и обработка продолжается

#### Scenario: Отсутствующий или недействительный токен

- **WHEN** заголовок `X-Webhook-Token` отсутствует или его хэш не найден в индексе
- **THEN** сервис возвращает HTTP 401 без дополнительной обработки

### Requirement: Приём вебхуков Prometheus Alertmanager

Сервис ingestion ДОЛЖЕН (SHALL) предоставлять POST-эндпоинт `/api/ingest/v1/webhook/alertmanager`, принимающий payload вебхука Alertmanager (формат v4) и подтверждать приём ответом HTTP 200 после успешной обработки всех алертов payload.

#### Scenario: Корректный payload Alertmanager

- **WHEN** Alertmanager отправляет POST с валидным payload на `/api/ingest/v1/webhook/alertmanager`
- **THEN** сервис нормализует, дедуплицирует и публикует каждый алерт, после чего возвращает HTTP 200

#### Scenario: Некорректный payload

- **WHEN** тело запроса не может быть разобрано как Alertmanager v4 JSON
- **THEN** сервис возвращает HTTP 400 с описанием ошибки

### Requirement: Приём вебхуков Grafana

Сервис ingestion ДОЛЖЕН (SHALL) предоставлять POST-эндпоинт `/api/ingest/v1/webhook/grafana`, принимающий payload legacy-вебхука Grafana (поля `state`, `ruleName`, `tags`, `message`) и подтверждать приём ответом HTTP 200 после успешной обработки.

#### Scenario: Корректный payload алерта Grafana

- **WHEN** Grafana отправляет POST с валидным вебхуком алерта (`state: alerting`) на `/api/ingest/v1/webhook/grafana`
- **THEN** сервис нормализует алерт как firing, обрабатывает его и возвращает HTTP 200

#### Scenario: Статус resolved в Grafana

- **WHEN** payload Grafana содержит `state: ok` или `state: paused`
- **THEN** сервис нормализует его как событие resolved-алерта

### Requirement: Публикация нормализованных алертов в RabbitMQ

Сервис ingestion ДОЛЖЕН (SHALL) публиковать каждый не-дедублицированный нормализованный алерт на exchange `alerts` в RabbitMQ (routing key `alert.received`) в виде JSON-сообщения **синхронно в рамках обработки HTTP-запроса**: успешный ответ вебхука означает, что алерты опубликованы.

#### Scenario: Успешная публикация

- **WHEN** нормализованный неповторяющийся алерт готов к публикации
- **THEN** сервис публикует AMQP-сообщение на exchange `alerts` и после обработки всех алертов payload возвращает HTTP 200

#### Scenario: RabbitMQ недоступен

- **WHEN** брокер RabbitMQ недоступен в момент публикации
- **THEN** сервис выполняет до 3 повторных попыток с экспоненциальной задержкой; при окончательном сбое удаляет dedup-ключ алерта (чтобы повторная отправка источником прошла дедупликацию), логирует структурированную ошибку и возвращает HTTP 503 вызывающей стороне
