## Why

`pkg/amqp.Publisher.publish` открывает новый AMQP-канал и сразу закрывает его на **каждое** опубликованное сообщение ([pkg/amqp/amqp.go:157](../../../pkg/amqp/amqp.go#L157)). `channel.open` и `channel.close` — синхронные команды протокола, то есть **два лишних сетевых round-trip'а к RabbitMQ на каждое сообщение** поверх самой публикации (находка аудита **P1**, [docs/audit/10-performance.md](../../../docs/audit/10-performance.md)). Это горячий путь всех продюсеров (ingestion — на каждый алерт, incident — на каждое изменение инцидента, escalation — на каждый trigger/exhaust) и самый дорогой структурный оверхед публикации под нагрузкой.

## What Changes

- Держать **один долгоживущий канал** на `Publisher`, защищённый мьютексом; переоткрывать его лениво только при ошибке публикации или закрытии канала. Это убирает оба round-trip'а из установившегося режима.
- AMQP-каналы не безопасны для конкурентной публикации, поэтому доступ к переиспользуемому каналу сериализуется мьютексом `Publisher`. Время удержания мьютекса — один `PublishWithContext` (без publisher confirms → возврат после записи в сокет, без round-trip к брокеру), поэтому сериализация дёшева в сравнении с убранными двумя RTT.
- Существующая логика 3× ретраев с backoff (`publishWithRetry`) и метрика `amqp_publish_total{exchange,result}` сохраняются без изменений.
- Добавить `Publisher.Close()` для аккуратного закрытия канала при остановке сервиса.
- Добавить бенчмарк публикации (live-брокер через `RABBITMQ_URL`) и приложить benchstat до/после к коммиту.
- **Не BREAKING:** wire-формат `Envelope`/payload, имена exchange/routing-key, API и схема БД не меняются. Publisher confirms намеренно **вне объёма** (вернули бы round-trip и изменили семантику надёжности) — в бэклог.

## Capabilities

### New Capabilities
<!-- Нет: чисто инфраструктурная перф-оптимизация без новой продуктовой capability. -->

### Modified Capabilities
<!-- Нет дельты: наблюдаемое поведение продуктовых capability не меняется (та же доставка событий, тот же wire-формат). Изменение локально для pkg/amqp; выигрыш подтверждается benchstat. -->

## Impact

- **Затронутый код:** `pkg/amqp/amqp.go` (`Publisher`), новый `pkg/amqp/amqp_bench_test.go`; точки остановки в `services/incident`, `services/escalation`, `services/ingestion` `cmd/server/main.go` (вызов `Publisher.Close()`).
- **Затронутые сервисы:** ingestion, incident, escalation (публикуют через `pkg/amqp.Publisher`). notification и scheduling не публикуют в шину — не затронуты.
- **События RabbitMQ:** `alert.received`, `incident.created`, `incident.updated`, `escalation.triggered`, `escalation.exhausted` — **wire-формат без изменений, не BREAKING**.
- **Зависимости:** новых внешних зависимостей нет. Опирается на переработанный в CH07 `pkg/amqp.Connection` (`Channel(ctx)`).
