# ADR-0018: Метрики конвейера, шинные золотые сигналы и алертинг

- Status: Accepted
- Date: 2026-06-14
- Change: pipeline-metrics-and-alerts
- Affected: pkg/metrics, pkg/amqp, pkg/db, services/ingestion, services/incident, services/escalation, services/notification

## Context

Аудит наблюдаемости ([docs/audit/07-observability.md](../audit/07-observability.md))
выявил, что событийный конвейер — то, ради чего система существует, — слеп для
метрик:

- **O2 (major):** нулевое покрытие метриками конвейера и шины. Есть только
  HTTP-фронт (`http_requests_total`, `http_request_duration_seconds`) и один
  доменный образец `ingestion_dedup_*`. Нет счётчиков принятых алертов, созданных
  инцидентов, запущенных/исчерпанных эскалаций, отправленных уведомлений; нет
  шинных сигналов (ack/nack/requeue, длительность обработки, publish-ошибки); нет
  метрик пула pgx. В связке с тихой смертью консьюмера (C1,
  [ADR-0015](0015-consumer-lifecycle-and-resilience.md)) нет даже метрики «жив ли
  конвейер».
- **R1 (perf/correctness):** HTTP-метрики метят сырой `r.URL.Path`, что взрывает
  кардинальность на per-tenant путях `/api/<svc>/v1/{tenant}/...`.

Находка **O5** (scrape-конфиг / alert-правила / дашборды) **вынесена из объёма
CH11** по решению владельца и будет сделана отдельно; этот ADR фиксирует
соглашение об именах метрик, которое O5 затем потребит.

Имена метрик и место их инструментирования — кросс-сервисное соглашение для
5-сервисного конвейера (как и каркас консьюмера в ADR-0015 и HTTP-bootstrap в
[ADR-0017](0017-shared-httpserver-and-readiness.md)), поэтому фиксируется ADR.

## Options considered

- **Шинные метрики централизованно в `pkg/amqp`, лейбл по `queue`/`exchange`.**
  Инструментируем `Consume.process` и `Publisher.Publish` один раз; метрики
  получают все консьюмеры/издатели. Низкая кардинальность (фикс. набор
  очередей/обменов), self-identifying. Принято.
- **Per-service шинные метрики `<service>_messages_processed_total`** (как в
  тексте аудита). Отклонено: возвращает в каждый `internal/consumer` копипасту
  цикла учёта, которую CH07 устранил в `pkg/amqp`; требует прокинуть имя сервиса
  в каждую ack/nack-ветку.
- **Лейбл `tenant_id` на конвейерных метриках** (по образцу `dedup`). Отклонено
  для конвейера: на multi-tenant потоке — потенциальный взрыв кардинальности;
  разрез по тенанту остаётся в структурных логах. У `dedup` лейбл уже есть и
  сохраняется.
- **Метрики пула pgx в `pkg/metrics`.** Отклонено: потянуло бы зависимость
  `pkg/metrics → pgxpool`. Коллектор живёт в `pkg/db` рядом с конфигом пула
  ([CH09/D4](0003-postgres-schema-per-service.md)).

## Decision

### Соглашение об именах и местах метрик

- **Шина (`pkg/amqp`):**
  - `amqp_messages_processed_total{queue,result}` — `result ∈ {ack, requeue, drop}`;
  - `amqp_message_processing_seconds{queue}` — гистограмма (`DefBuckets`);
  - `amqp_publish_total{exchange,result}` — `result ∈ {ok, error}`.
  Инструментируются в `Consume.process` (ack/nack-ветки) и `Publisher.Publish`.
- **Доменные метрики (паттерн `dedup`: package-level `var` + `init()` с
  `MustRegister`):**
  - ingestion: `ingestion_alerts_received_total{source}`;
  - incident: `incident_incidents_created_total`, `incident_incidents_resolved_total`;
  - escalation: `escalation_triggered_total`, `escalation_advanced_total`,
    `escalation_exhausted_total`, `escalation_getoncall_failures_total`,
    gauge `escalation_backlog`;
  - notification: `notification_sent_total{channel,result}`,
    `notification_rate_limited_total{channel}`.
- **Пул pgx (`pkg/db`):** `RegisterPoolMetrics(service, pool)` — коллектор поверх
  `pgxpool.Stat()`, серии `db_pool_*{service}`; вызывается из каждого `main`.
- Над каждым объявлением метрики — комментарий с PromQL/назначением (соглашение
  скила `golang-observability`).

### R1 — RoutePattern вместо URL.Path

`pkg/metrics.Middleware` метит запрос по `chi.RouteContext(r.Context()).RoutePattern()`
(читается после `ServeHTTP`). Пустой шаблон (404/неузнанный путь) сводится к лейблу
`"other"`. Меняется значение лейбла `path` существующих HTTP-метрик — не имена
метрик. В репозитории ещё нет дашбордов/алертов по `path` (это и есть O5), так что
обратная несовместимость лейбла никого не ломает.

Контракты API, события RabbitMQ и схема БД **не меняются** — решение про
инструментирование и алертинг, не про контракты (не BREAKING). Capability-дельты
нет (infra/observability, согласовано с владельцем).

## Consequences

- Появляется операционная видимость конвейера: «доходят ли уведомления»,
  «не растёт ли очередь/backlog», «жив ли консьюмер», насыщение пула — всё
  становится метриками и алертами.
- `pkg/amqp` — единственная точка эволюции шинных метрик; новые консьюмеры/издатели
  получают золотые сигналы бесплатно. Доменные метрики добавляются по паттерну
  `dedup` в доменных пакетах.
- Конвейерные метрики не разрезаются по `tenant_id` — per-tenant анализ только
  через логи; это осознанный размен ради кардинальности.
- Значение лейбла `path` HTTP-метрик меняется на RoutePattern — будущие
  дашборды/алерты строятся уже на шаблонах роутов.
- **O5** (scrape/alerts/дашборды) делается отдельным чейнджем и опирается на
  имена метрик, зафиксированные здесь.
- Открывает CH12 (корреляция логов `request_id`/`trace_id`) и CH13
  (OpenTelemetry); CH15 (ingestion-throughput) получает метрики для замера эффекта.
