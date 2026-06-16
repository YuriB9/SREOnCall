## Why

На пути приёма вебхука каждый алерт обрабатывается строго последовательно: цикл `Redis SETNX → Postgres INSERT → AMQP publish` по одному алерту прямо в HTTP-запросе. Вебхук Alertmanager штатно несёт десятки алертов в одном теле, поэтому отправитель блокируется на `N × (RTT Redis + RTT Postgres + publish)` и может упереться в собственный таймаут, ретраить и дублировать нагрузку. Дополнительно: построчные INSERT'ы лейблов в incident (N round-trip'ов на N лейблов) и двойной `json.Marshal` структуры `Alert` на каждый алерт. Находки P2, P3, P5 аудита производительности (`docs/audit/10-performance.md`).

Это **perf-чейндж без смены наблюдаемого поведения**: HTTP-ответы, формат событий и семантика дедупликации не меняются. Цель — снять число последовательных сетевых round-trip'ов на пути приёма.

## What Changes

- **P2 — батчинг приёма в ingestion.** `processAlerts` переписан с поалертного цикла на три групповые операции на одно тело вебхука:
  - пайплайн дедупликации — все `SETNX`/`DEL` в один round-trip Redis (новый `dedup.Deduplicator.Classify`);
  - батч-INSERT `raw_alerts` через `pgx.Batch` вместо INSERT в цикле;
  - публикация неподавленных алертов на переиспользуемом канале (введён в CH14).
  Порядок алертов, семантика дедупа и откат дедуп-ключа при ошибке публикации сохранены.
- **P3 — multi-row upsert лейблов в incident.** `mergeLabels` делает один `INSERT ... SELECT FROM unnest($::text[], $::text[])` вместо N построчных INSERT'ов. Покрывает оба вызова — `MergeLabels` и `CreateIncidentTx` (общий приватный helper).
- **P5 — единая сериализация `Alert` в ingestion.** Структура `Alert` маршалится один раз на алерт; готовый `json.RawMessage` переиспользуется и для JSONB-колонки `raw_alerts.payload`, и для payload конверта (`Wrap(json.RawMessage)` не реэнкодит структуру).
- Бенчмарки горячих путей (`BenchmarkProcessAlerts`, `BenchmarkMergeLabels`) + benchstat до/после — обязательное требование аудита для каждой оптимизации.

Объём строго P2/P3/P5. P2(б) (воркер-пул консьюмеров, `errgroup.SetLimit` + `Qos(n)`) уже закрыт в CH07/C8 — здесь не трогается, только верифицируется. Async-вариант приёма (ответ `202` + очередь приёма) **сознательно отклонён**: он меняет гарантию доставки и синхронный контракт вебхука.

## Capabilities

### New Capabilities
<!-- Нет: новых продуктовых capability не вводится. -->

### Modified Capabilities
<!-- Нет: наблюдаемое поведение продуктовых capability (приём/дедуп алертов, инциденты) не меняется — это инфраструктурно-перформанс-рефактор без дельты спека (прецедент: CH14 bus-publish-perf, CH09 store-layering). -->

## Impact

- **Затронутые сервисы:**
  - **services/ingestion** — `internal/handler` (`processAlerts`/`processOne`), `internal/dedup` (Cache + Deduplicator), `internal/store` (батч-сохранение), `internal/publisher` (публикация готового payload).
  - **services/incident** — `internal/store` (`mergeLabels`).
- **События RabbitMQ:** `alert.received` (exchange `alerts`) — батчинг только на стороне продюсера, **wire-формат конверта и payload идентичны**. **НЕ BREAKING** для консьюмеров.
- **Схема БД:** без изменений, миграций нет. Поведение `raw_alerts` (append-only аудит) и `incident_labels` (upsert) сохранено.
- **API:** без изменений — те же коды ответов (200 / 400 / 503), тот же контракт вебхука.
- **Зависимости:** опирается на переиспользуемый канал публикации из CH14 и вынесенный store-слой ingestion из CH09; метрики из CH11 используются для замера.
