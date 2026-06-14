## Why

Логи невозможно скоррелировать с конкретным запросом или трассой (находка
аудита **O4**): в коде **ноль** `*Context`-логов — все вызовы вида
`logger.Error("...", "err", err)` пишут без `ctx`, поэтому `request_id`,
который ставит `chiMiddleware.RequestID` (теперь везде — единый middleware-набор
из CH10/O6), **не попадает ни в одну запись**. Идентификатор в контексте есть, а
в логах его нет — при инциденте нельзя собрать все записи одного запроса в
агрегаторе (Loki/ELK).

Отдельно (находка **E5**): ключ ошибки в `slog` исторически плавал между `"err"`
и `"error"`, что для агрегатора — два разных поля и фрагментация группировки.
К текущему main расхождение уже устранено профильными рефакторингами (CH05–CH11):
в продовом коде **0** slog-вызовов с ключом `"error"` и **~100** с `"err"`
(оставшиеся литералы `"error"` — это JSON-тела HTTP-ответов, контракт API, их
трогать нельзя). Ценность CH12 для E5 — **закрепить инвариант линтером**, чтобы он
не отъехал снова.

Это закрывает находки **O4 и E5** из
[docs/audit/07-observability.md](../../../docs/audit/07-observability.md) и
[docs/audit/04-error-handling.md](../../../docs/audit/04-error-handling.md).
Зависимость CH10 (единый `RequestID`-middleware во всех 5 сервисах через
`pkg/httpserver`) уже в main — есть откуда брать `request_id`. Часть `trace_id`
подключится после CH13 (distributed-tracing) — хендлер логов делается с заделом
под неё, но сам `trace_id` в этом чейндже не вводится.

## What Changes

- **O4 — context-aware slog-хендлер в `pkg/logger`:** `New(level)` оборачивает
  JSON-хендлер декоратором, который в `Handle(ctx, record)` достаёт `request_id`
  из контекста (`chiMiddleware.GetReqID`) и добавляет атрибутом к каждой записи.
  Точка расширения под `trace_id`/`span_id` (CH13) помечена комментарием. Так как
  не-Context-методы `slog` передают `context.Background()`, `request_id` появится
  только у вызовов, переведённых на `*Context`.
- **O4 — перевод логирующих вызовов на `*Context`-варианты** там, где `ctx`
  доступен в области видимости: хендлеры (`r.Context()`), консьюмеры/нотифаер/
  эскалятор/монитор (`ctx`-параметр), `pkg/auth` resolve, `pkg/amqp`
  consume/publish. Стартовые логи в `main.go` (до обработки запросов) остаются
  не-Context — `request_id` там бессмысленен.
- **E5 — закрепление единого ключа `"err"` линтером:** включить `sloglint` в
  `.golangci.yml` с `forbidden-keys: ["error"]` (только для slog-вызовов; JSON-тела
  ответов `{"error": ...}` это не затрагивает). Гейт `only-new-issues` не даст
  вернуть `"error"`-ключ в новый код.

Изменение наблюдаемого поведения продуктовых capability нет — это infra/
observability (прецедент CH07/CH10/CH11, архив с `--skip-specs`). ADR не
создаётся: решение не относится к значимым архитектурным категориям из
`openspec/config.yaml` (транспорт/хранилище/auth/раскладка/границы/версионирование
событий).

## Capabilities

### New Capabilities
<!-- Нет. Корреляция логов не вводит продуктовую capability. -->

### Modified Capabilities
<!-- Нет дельты. Инъекция request_id в записи логов и линт-гард ключа ошибки не
     меняют наблюдаемое поведение продуктовых capability — infra/observability. -->

## Impact

**Затронутые сервисы** (перевод логов на `*Context`):
- **ingestion** — `internal/handler`.
- **incident** — `internal/handler`, `internal/consumer`.
- **escalation** — `internal/handler`, `internal/escalator`, `internal/monitor`,
  `internal/consumer`.
- **notification** — `internal/handler`, `internal/notifier`, `internal/consumer`.
- **scheduling** — `internal/handler`.

**Общие модули:** `pkg/logger` (новый context-хендлер), `pkg/auth` (resolve),
`pkg/amqp` (логи consume/publish). `.golangci.yml` (+`sloglint`).

**Деплой:** не затрагивается. В JSON-логах появляется поле `request_id` у записей
из request-scope — операционно полезно, форматы алертов на логах не ломает.

**События RabbitMQ:** не затрагиваются. **НЕ BREAKING.**

**API:** без изменений. JSON-ответы `{"error": ...}` сохраняются как есть.
