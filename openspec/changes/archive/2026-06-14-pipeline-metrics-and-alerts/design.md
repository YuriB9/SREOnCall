## Context

Аудит наблюдаемости ([07-observability.md](../../../docs/audit/07-observability.md))
зафиксировал: фундамент верный (slog, HTTP-гистограммы, авто-рантайм-метрики,
`ingestion_dedup_*` как образец), но событийный конвейер не инструментирован
(O2), а HTTP-метрики метят сырой `r.URL.Path` (R1, взрыв кардинальности на
`/api/.../{tenant}/...`). Находка O5 (scrape/alerts/дашборды) вынесена из объёма
CH11 — делается отдельно; CH11 готовит метрики, которые O5 затем потребит.

Почва готова предыдущими чейнджами: CH10 ввёл общий `pkg/httpserver` с цепочкой
middleware (`RequestID → Recoverer → metrics`) и `Probe`-сигнал живости
консьюмера; CH07 — supervisor-петлю `pkg/amqp.Consume` и `Publisher`; CH09 —
конфиг пула pgx (`pgxpool.Stat()` теперь осмыслен). Доменные пакеты
(`internal/notifier`, `internal/escalator`, `internal/consumer`,
`internal/handler`) — естественные точки доменных метрик.

## Goals / Non-Goals

**Goals:**
- Золотые сигналы шины для всех консьюмеров/издателей — **одной** точкой
  инструментирования в `pkg/amqp` (не копипастой по сервисам).
- Доменные метрики ключевых стадий конвейера (приём → инцидент → эскалация →
  уведомление) по образцу `dedup`.
- Метрики пула pgx из общего `pkg/db`.
- Фикс кардинальности HTTP-метрик (R1).

**Non-Goals:**
- **O5 (scrape-конфиг / alert-правила / дашборды) — вне объёма CH11**, делается
  отдельно. CH11 только экспонирует метрики на `/metrics`.
- Распределённая трассировка / OpenTelemetry (O3 → CH13).
- Корреляция логов `request_id`/`trace_id` (O4 → CH12).
- DLQ, изменение wire-формата событий, новые поля в `Envelope`.

## Decisions

### D1. Шинные метрики — централизованно в `pkg/amqp`, лейбл по queue/exchange
Аудит предлагал `<service>_messages_processed_total`. Вместо per-service имён
вводим **технические** метрики в самом `pkg/amqp`, self-identifying по лейблам
`queue`/`exchange`:
- `amqp_messages_processed_total{queue,result}` — `result ∈ {ack, requeue, drop}`;
- `amqp_message_processing_seconds{queue}` (гистограмма, `DefBuckets`);
- `amqp_publish_total{exchange,result}` — `result ∈ {ok, error}`.

Инструментируется в `Consume.process` (ack/nack-ветки) и `Publisher.Publish`.
**Почему так, а не per-service:** консьюмер-цикл живёт в `pkg/amqp`, проброс
имени сервиса в каждую ack-ветку — лишний параметр и риск рассинхрона; `queue` и
`exchange` уже однозначно идентифицируют поток (`alerts.incident`,
`escalation.notification`, exchange `alerts`/`incidents`/`escalations`). Низкая
кардинальность (фикс. набор очередей/обменов). Отклонено: дублировать счётчики в
каждом `internal/consumer` — это ровно копипаста, которую CH07 устранил для
самого цикла.

### D2. Доменные метрики — в доменных пакетах, паттерн `dedup`
Package-level `prometheus.NewCounterVec`/`Gauge` + `init()` с `MustRegister`,
ровно как `ingestion/dedup`. Набор:
- **ingestion**: `ingestion_alerts_received_total{source}` (в `handler` на
  успешной нормализации). Publish-ошибки уже покрыты `amqp_publish_total`.
- **incident**: `incident_incidents_created_total`,
  `incident_incidents_resolved_total` (в `consumer` после успешной транзакции).
- **escalation**: `escalation_triggered_total`, `escalation_advanced_total`,
  `escalation_exhausted_total`, `escalation_getoncall_failures_total`
  (в `escalator`); gauge `escalation_backlog` — длина пачки `ListExpiredStates`
  на каждом тике (в `monitor`).
- **notification**: `notification_sent_total{channel,result}` (result
  delivered|failed), `notification_rate_limited_total{channel}` (в `notifier`,
  на точках записи `NotificationLog`).

Лейбл `tenant_id` **не** навешиваем на конвейерные метрики (в отличие от
`dedup`, где он уже есть) — на multi-tenant потоке это потенциальный взрыв
кардинальности; разрез по тенанту остаётся в логах. `source`/`channel` —
ограниченные множества, безопасны.

### D3. Метрики пула pgx — коллектор в `pkg/db`
`pkg/db.RegisterPoolMetrics(service string, pool *pgxpool.Pool)` регистрирует
`prometheus.Collector`, который на скрейпе читает `pool.Stat()` и отдаёт
`db_pool_*{service}` (acquired/idle/total/max, acquire-count, acquire-wait).
`pkg/db` уже импортирует `pgxpool` — метрики живут рядом с конфигом пула (D4).
Вызывается из 5 `main.go` сразу после `NewPool`. Отклонено: класть в
`pkg/metrics` — потянуло бы зависимость `pkg/metrics → pgxpool`.

### D4. R1 — RoutePattern вместо URL.Path
В `pkg/metrics.Middleware` после `next.ServeHTTP` читаем
`chi.RouteContext(r.Context()).RoutePattern()`. Шаблон известен только **после**
матчинга роута, поэтому лейбл вычисляется после обработки (для duration —
наблюдаем по завершении). Пустой шаблон (404/неузнанный) → лейбл `"other"`,
чтобы не плодить серии по мусорным путям. `pkg/metrics` уже косвенно в графе chi
(используется из `pkg/httpserver`), прямой импорт `chi/v5` добавляется в
`pkg/metrics`.

## Risks / Trade-offs

- **Двойная регистрация метрик в тестах** (`prometheus.MustRegister` паникует
  при повторной регистрации) → package-level `var` + `init()` регистрируются
  один раз на процесс; тесты, поднимающие компонент многократно, используют те
  же глобальные коллекторы — как уже устроено в `dedup`.
- **Отсутствие лейбла `tenant_id` на конвейерных метриках** → теряем
  per-tenant-разрез в метриках; **митигировано** тем, что разрез по тенанту
  остаётся в структурных логах, а кардинальность важнее для алертинга.
- **R1 меняет значение лейбла `path`** у существующих HTTP-метрик → старые
  дашборды/алерты на `path="/api/..."` перестали бы матчиться; **митигировано**
  тем, что в репозитории дашбордов/алертов по `path` ещё нет (это O5, отдельно),
  фиксируется в ADR-0018.

## Migration Plan

- Метрики аддитивны: новые серии появляются после деплоя, старые
  (`http_*`, `ingestion_dedup_*`) сохраняются. Откат — обычный откат образа.
- Wire-формат событий не меняется — сообщения в очередях и от старых версий
  обрабатываются как прежде; смешанный деплой безопасен.

## Open Questions

- Нет. Решения по capability-дельте (нет) и ADR (ADR-0018) согласованы с
  владельцем на старте чейнджа.
