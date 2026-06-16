## Why

`tenantcache.Cache` корректно **не** держит мьютекс во время сетевого `fetch` ([cache.go:36-55](../../../services/notification/internal/tenantcache/cache.go#L36-L55)) — гонок данных нет. Но из этого следуют два структурных минуса (находка аудита **C7**, [docs/audit/02-concurrency.md §C7](../../../docs/audit/02-concurrency.md)):

1. **Cache stampede.** При промахе/протухании ключа популярного тенанта N параллельных доставок одновременно увидят отсутствие записи и сделают N одновременных запросов в scheduling. Нет дедупликации одновременных fetch'ей.
2. **Неограниченный рост памяти.** Структура названа «LRU-style», но вытеснения нет: TTL лишь перезаписывает запись при следующем `Get` того же ключа. Тенанты, переставшие слать события, остаются в `data` навсегда.

## What Changes

- Обернуть `fetcher` в `golang.org/x/sync/singleflight.Group`: одновременные промахи по одному `tenantSlug` дедуплицируются в **один** вызов scheduling, результат раздаётся всем ожидающим (skill `golang-concurrency`: «Caching expensive computations → singleflight»).
- Добавить **вытеснение протухших ключей**: фоновая периодическая чистка `data` от записей с истёкшим TTL (запускается из `New`, останавливается по `ctx`). Это убирает неограниченный рост map при стабилизации/смене набора активных тенантов.
- Сохранить текущую семантику `Get`: успешный результат кешируется на TTL; при ошибке fetch запись не пишется; лок не держится через I/O.
- Юнит-тесты на дедупликацию параллельных промахов (один fetch на N горутин) и на вытеснение протухших ключей.
- **Не BREAKING:** сигнатура `Get` не меняется; API, события RabbitMQ и схема БД не затрагиваются. Меняется только внутреннее поведение кэша notification.

## Capabilities

### New Capabilities
<!-- Нет: чисто внутренняя конкурентно-перф-оптимизация кэша, новой продуктовой capability нет. -->

### Modified Capabilities
<!-- Нет дельты: наблюдаемое поведение notification-dispatch не меняется (та же конфигурация тенанта, тот же TTL-кэш с точки зрения вызывающего). Изменение локально для services/notification/internal/tenantcache. Спеки кэш не описывают (проверено: openspec/specs/notification-dispatch не упоминает кэш/TTL/stampede). -->

## Impact

- **Затронутый код:** `services/notification/internal/tenantcache/cache.go` (singleflight + eviction), новый `services/notification/internal/tenantcache/cache_test.go`; `services/notification/cmd/server/main.go` (проброс `ctx` в `tenantcache.New` для остановки фоновой чистки).
- **Затронутые сервисы:** **notification** (единственный потребитель `tenantcache`). ingestion, incident, escalation, scheduling не используют кэш — не затронуты.
- **Косвенный эффект:** меньше синхронных запросов notification→scheduling под пиком (`GetTenantNotificationConfig`); ограниченный рост памяти кэша.
- **События RabbitMQ:** не затрагиваются (кэш на синхронном S2S-пути notification→scheduling, не на шине).
- **Зависимости:** новых внешних нет — `golang.org/x/sync` уже прямая зависимость модуля notification (errgroup, CH07); `singleflight` в том же модуле.
