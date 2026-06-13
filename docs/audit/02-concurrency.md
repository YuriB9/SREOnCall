# Аудит SREOnCall — Область 2: Конкурентность

Дата: 2026-06-13
Область: горутины, консьюмеры RabbitMQ, утечки, гонки, отмена через context.
Применённые скилы: `golang-concurrency`, `golang-context`.

Объём конкурентного кода невелик и локализован: фоновые горутины запускаются только в `cmd/server/main.go` каждого сервиса (консьюмер + у escalation ещё монитор), плюс две точки разделяемого состояния (`pkg/amqp.Connection`, `notification/tenantcache.Cache`). Гонок данных (`-race`-класса) не обнаружено — оба разделяемых стейта корректно защищены мьютексом. Основные проблемы — в **жизненном цикле горутин**: отсутствие graceful-drain, отсутствие переподключения консьюмеров и неотменяемые блокировки.

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| C1 | **critical** | Консьюмеры не переподключаются: при разрыве соединения с RabbitMQ горутина консьюмера завершается, сервис остаётся «healthy», но навсегда перестаёт обрабатывать события | [escalation/consumer.go:57](services/escalation/internal/consumer/consumer.go#L57), [incident/consumer.go:73](services/incident/internal/consumer/consumer.go#L73), [notification/consumer.go:46](services/notification/internal/consumer/consumer.go#L46); запуск: [incident/main.go:133](services/incident/cmd/server/main.go#L133) |
| C2 | **major** | Нет graceful-drain фоновых горутин: `main` не ждёт консьюмер/монитор (нет `WaitGroup`/`errgroup`), `amqpConn` нигде не закрывается → на SIGTERM обработка in-flight сообщения убивается на полпути | все `*/cmd/server/main.go`; нет `defer amqpConn.Close()` |
| C3 | **major** | In-flight доставка обрабатывается с уже отменённым `ctx` при shutdown → многошаговые незавершённые записи в БД + Nack/requeue → дубли/частичное состояние | [incident/consumer.go:92-110](services/incident/internal/consumer/consumer.go#L92-L110) |
| C4 | **major** | `Connection.Channel()` держит мьютекс через `dialWithRetry` со `time.Sleep` до ~31 c → при сбое брокера все паблишеры сериализуются за залоченным на 31 c мьютексом | [pkg/amqp/amqp.go:43-59](pkg/amqp/amqp.go#L43-L59) |
| C5 | **minor** | Бэкофф-ретраи через `time.Sleep` игнорируют `ctx` (диспетчеры + reconnect) | [mattermost.go:42](services/notification/internal/dispatcher/mattermost.go#L42), [email.go:88](services/notification/internal/dispatcher/email.go#L88), [amqp.go:58](pkg/amqp/amqp.go#L58) |
| C6 | **minor** | `Publish` передаёт `context.Background()` в `PublishWithContext` — публикация неотменяема, переданный `ctx` игнорируется на самой I/O-операции | [pkg/amqp/amqp.go:82-93](pkg/amqp/amqp.go#L82-L93) |
| C7 | **minor** | `tenantcache.Cache`: нет защиты от cache stampede (singleflight) и нет вытеснения протухших ключей → дублирующиеся fetch'и и неограниченный рост map | [tenantcache/cache.go:36-55](services/notification/internal/tenantcache/cache.go#L36-L55) |
| C8 | **minor** | `Qos(10)` при строго последовательной обработке в цикле — prefetch не даёт параллелизма, эффект нулевой | [incident/consumer.go:59](services/incident/internal/consumer/consumer.go#L59) и др. |

---

## Детализация

### C1 — Консьюмеры не переживают переподключение к RabbitMQ — **critical**

`pkg/amqp.Connection` документирован как «with basic reconnection support», но переподключение работает **только для паблишеров**: `Publisher.publish` берёт свежий канал на каждую публикацию через `Channel()`, который при закрытом соединении передоберётся ([amqp.go:43-50](pkg/amqp/amqp.go#L43-L50)). Консьюмеры устроены иначе — канал и поток доставок (`msgs`) открываются один раз в начале `Run`:

```go
msgs, err := ch.Consume(...)        // incident/consumer.go:64
for {
    select {
    case <-ctx.Done(): return nil
    case msg, ok := <-msgs:
        if !ok { return fmt.Errorf("consumer: channel closed") } // :73
        ...
    }
}
```

Когда соединение с брокером рвётся (рестарт RabbitMQ, сетевой блип), `amqp091` закрывает `msgs` → `ok == false` → `Run` возвращает ошибку. В `main` эта ошибка только **логируется**, горутина завершается и больше не перезапускается:

```go
go func() {
    if err := cons.Run(ctx, amqpConn); err != nil && ctx.Err() == nil {
        logger.Error("consumer error", "err", err) // и всё — горутина мертва
    }
}()
```

([incident/main.go:133](services/incident/cmd/server/main.go#L133), аналогично [escalation/main.go:80](services/escalation/cmd/server/main.go#L80), [notification/main.go:90](services/notification/cmd/server/main.go#L90)).

Последствие: после любого мигания брокера сервис остаётся живым (`/healthz` и `/readyz` отвечают 200 — см. область 1, они статические), HTTP-API работает, но **весь событийный конвейер этого сервиса встаёт навсегда** до ручного рестарта пода. Алерты перестают превращаться в инциденты, эскалации не запускаются, уведомления не уходят — без единого failed-healthcheck. Это самый тяжёлый дефект в конкурентной части.

**Фикс.** Обернуть `cons.Run` в supervisor-петлю с backoff и отменой по `ctx`:

```go
go func() {
    for ctx.Err() == nil {
        if err := cons.Run(ctx, amqpConn); err != nil && ctx.Err() == nil {
            logger.Error("consumer stopped, restarting", "err", err)
            select {
            case <-ctx.Done():
            case <-time.After(backoff): // с экспонентой
            }
        }
    }
}()
```

Дополнительно — завязать `/readyz` на состояние консьюмера (NotifyClose канал соединения), чтобы k8s видел деградацию. По скилу `golang-concurrency`: «every goroutine must have a clear exit *and* restart story» — здесь exit есть, а recovery нет.

---

### C2 — Нет graceful-drain фоновых горутин на shutdown — **major**

Ни в одном `main` нет ни `sync.WaitGroup`, ни `errgroup`, ни `defer amqpConn.Close()` (закрываются только `pool` и `rdb` — [incident/main.go:43](services/incident/cmd/server/main.go#L43)). Последовательность завершения такая:

```go
<-ctx.Done()                          // пришёл SIGTERM
shutCtx, cancel := context.WithTimeout(...)
_ = srv.Shutdown(shutCtx)             // дождались только HTTP
}                                     // main возвращается → процесс умирает
```

Консьюмер и монитор (`go mon.Run(ctx)` — [escalation/main.go:92](services/escalation/cmd/server/main.go#L92)) запущены как fire-and-forget. На SIGTERM `ctx` отменяется, но `main` **не дожидается** их завершения: после `srv.Shutdown` он сразу выходит, и рантайм убивает фоновые горутины в произвольной точке. Если консьюмер в этот момент внутри `handle()` (запись в БД, публикация), операция обрывается на полуслове, `Ack`/`Nack` не отправляется. Поскольку `amqpConn` не закрывается явно и корректно, неподтверждённые сообщения возвращаются брокером только по таймауту/обрыву TCP.

По скилу `golang-concurrency` (чеклист «Can I wait for it?») каждая фоновая горутина должна джойниться. Сейчас нарушено для всех консьюмеров и монитора.

**Фикс.** Собрать фоновые горутины в `errgroup` (или `WaitGroup`) и дождаться их после `srv.Shutdown`, затем закрыть соединение:

```go
g, gctx := errgroup.WithContext(ctx)
g.Go(func() error { return cons.Run(gctx, amqpConn) })
g.Go(func() error { mon.Run(gctx); return nil })
...
<-ctx.Done()
srv.Shutdown(shutCtx)
_ = g.Wait()            // дренаж in-flight
_ = amqpConn.Close()    // явное закрытие
```

---

### C3 — In-flight доставка обрабатывается с отменённым контекстом — **major**

Консьюмеры пробрасывают свой `ctx` (с временем жизни = сервис) прямо в обработку сообщения: `c.handle(ctx, msg)` → `handleFiring(ctx, ...)`. На shutdown этот `ctx` отменяется, и **текущее** сообщение дообрабатывается уже с `Done`-контекстом. У incident `handleFiring` делает несколько **отдельных, не обёрнутых в транзакцию** записей:

```go
c.store.CreateIncident(ctx, inc)      // incident/consumer.go:148
c.store.MergeLabels(ctx, ...)         // :158
c.store.AppendHistory(ctx, ...)       // :163
c.store.AttachAlert(ctx, ...)         // :178
```

Если `ctx` отменяется между этими вызовами, pgx прервёт операцию посередине → инцидент создан, но без labels/alert; затем `handle` получит ошибку и сделает `Nack(false, true)` (requeue) → сообщение переобработается и создаст **дубликат** инцидента (поиск open-инцидента по group key мог ещё не зафиксироваться). Это пересечение конкурентности (отмена) и отсутствия транзакции (область БД), но триггер здесь — отмена контекста на остановке.

**Фикс (концептуально из `golang-context`).** Для фоновой обработки, которая должна доводиться до конца после сигнала остановки, использовать «drain-контекст», не наследующий отмену сервиса, но с собственным таймаутом:

```go
func (c *Consumer) handle(parent context.Context, msg amqp091.Delivery) {
    ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
    defer cancel()
    ...
}
```

Вместе с C2 (дождаться завершения handle) это даёт корректный at-least-once без обрыва на полуслове. Ортогонально — обернуть многошаговую запись incident в одну транзакцию (вынесу в область БД).

---

### C4 — Мьютекс удерживается через `time.Sleep` до 31 секунды — **major**

`Connection.Channel()` берёт `c.mu` и под ним вызывает `dialWithRetry`, который спит между попытками:

```go
func (c *Connection) Channel() (*amqp.Channel, error) {
    c.mu.Lock()
    defer c.mu.Unlock()                 // amqp.go:45 — лок держится до конца
    if c.conn == nil || c.conn.IsClosed() {
        if err := c.dialWithRetry(5); err != nil { ... }   // :47
    }
    return c.conn.Channel()
}

func (c *Connection) dialWithRetry(attempts int) error {
    for i := range attempts {
        ...
        time.Sleep(delay)               // :58 — 1+2+4+8+16 ≈ 31 c суммарно
    }
}
```

Анти-паттерн из `golang-concurrency` («Mutex held across I/O» / «never hold across I/O»). При недоступном брокере **первый** же паблишер уходит в `dialWithRetry` и держит `c.mu` до 31 секунды; все остальные `Channel()` (другие паблишеры, метрики, healthchecks, если бы они её дёргали) блокируются на мьютексе на это время, хотя могли бы быстро получить ошибку. Реконнект из параллельной нагрузки полностью сериализуется.

**Фикс.** Не держать `mu` во время дозвона/сна: под локом только читать/менять указатель `conn`; саму попытку реконнекта выполнять без лока (через `singleflight` либо двойную проверку), а `time.Sleep` заменить на `select { case <-ctx.Done(); case <-time.After(delay) }` (см. C5). И прокинуть `ctx` в `Channel()`.

---

### C5 — Бэкофф через `time.Sleep` неотменяем — **minor**

Ретрай-циклы спят «глухим» `time.Sleep`, игнорируя любую отмену:

- mattermost: `time.Sleep(time.Duration(1<<attempt) * time.Second)` — [mattermost.go:42](services/notification/internal/dispatcher/mattermost.go#L42);
- email: то же — [email.go:88](services/notification/internal/dispatcher/email.go#L88);
- amqp reconnect: [amqp.go:58](pkg/amqp/amqp.go#L58).

Сам HTTP-запрос диспетчера использует `ctx` (хорошо), но при shutdown горутина-консьюмер, отправляющая уведомление, всё равно зависнет в `Sleep` до 2 c между попытками, игнорируя отменённый контекст — что усугубляет C2/C3. Для сравнения, `Publisher.Publish` сделан правильно — там `select { <-ctx.Done(); <-time.After(delay) }` ([amqp.go:90-94](pkg/amqp/amqp.go#L90-L94)); этот же приём нужно применить везде.

**Фикс.** Заменить `time.Sleep(d)` на отменяемое ожидание:

```go
select {
case <-ctx.Done():
    return ctx.Err()
case <-time.After(d):
}
```

---

### C6 — `Publish` игнорирует переданный контекст на самой публикации — **minor**

Внешний ретрай-цикл `Publish` корректно реагирует на `ctx`, но фактический вызов брокера выполняется с `context.Background()`:

```go
return ch.PublishWithContext(
    context.Background(),   // amqp.go:84 — а не переданный ctx
    exchange, routingKey, false, false, ...)
```

([pkg/amqp/amqp.go:82-93](pkg/amqp/amqp.go#L82-L93)). Дедлайн/отмена вызывающего на саму операцию публикации не распространяются — нарушение базового правила `golang-context` («propagate the same context»). На практике с подтверждениями выключенными эффект мал, но это скрытый обрыв цепочки отмены.

**Фикс.** Передавать `ctx` из сигнатуры `publish(ctx, ...)` в `PublishWithContext(ctx, ...)`.

---

### C7 — `tenantcache`: cache stampede и неограниченный рост — **minor**

Гонок нет: `Get` корректно не держит мьютекс во время сетевого `fetch` ([cache.go:43-48](services/notification/internal/tenantcache/cache.go#L43-L48)) — это правильно. Но из этого следуют два структурных минуса:

1. **Cache stampede.** При промахе по популярному тенанту N параллельных доставок одновременно увидят отсутствие/протухание ключа и сделают N одновременных запросов в scheduling. Нет `singleflight` для дедупликации.
2. **Рост памяти.** Структура названа «LRU-style», но вытеснения нет: TTL только перезаписывает запись при следующем `Get` того же ключа; тенанты, которые перестали слать события, остаются в `data` навсегда.

**Фикс.** Обернуть `fetcher` в `golang.org/x/sync/singleflight.Group` (skill `golang-concurrency`: «Caching expensive computations → singleflight») и периодически чистить протухшие ключи (или взять готовый TTL-кэш). При текущем числе тенантов не критично, но на масштабе — да.

---

### C8 — `Qos(10)` бесполезен при последовательной обработке — **minor**

Все консьюмеры выставляют prefetch `ch.Qos(10, 0, false)`, но обрабатывают сообщения строго по одному: `handle()` вызывается синхронно в теле `for`-`select`, следующий `msg` не читается, пока текущий не подтверждён. Prefetch 10 означает лишь «до 10 неподтверждённых», но реально в работе всегда одно — параллелизма ноль.

Это не баг, а несоответствие намерения и реализации: либо обработка должна идти пулом воркеров (тогда prefetch осмыслен), либо prefetch стоит выставить в 1, чтобы не «придерживать» 9 сообщений у мёртвого по C1 консьюмера.

**Фикс (если нужна пропускная способность).** Bounded-пул через `errgroup.SetLimit(n)`: читать из `msgs` и раздавать в воркеры, prefetch = размеру пула. Иначе — снизить `Qos` до 1 и задокументировать последовательную семантику.

---

## Что сделано хорошо (для контекста)

- Оба разделяемых состояния защищены корректно: `tenantcache.Cache` (мьютекс, лок не держится через I/O) и `pkg/amqp.Connection` (мьютекс вокруг указателя соединения) — **гонок данных нет**.
- `monitor.Run` — образцовый воркер на тикере: `time.NewTicker` + `defer Stop()` + `select` с `ctx.Done()` ([monitor.go:31-41](services/escalation/internal/monitor/monitor.go#L31-L41)), без `time.After` в цикле.
- Консьюмеры и монитор честно слушают `ctx.Done()` в `select` — отмена доходит (проблема не в отсутствии отмены, а в отсутствии ожидания их завершения, C2).
- `Publisher.Publish` использует отменяемый backoff (`select` на `ctx.Done`) — правильный образец, который надо растиражировать (C5).
- Хендлеры не плодят фоновых горутин; вся конкурентность сосредоточена в `main` и предсказуема.

---

## Рекомендованный порядок исправлений

1. **C1** — supervisor-петля переподключения консьюмеров: устраняет «тихую смерть» конвейера (наивысший приоритет, critical).
2. **C2 + C3** — `errgroup` для джойна фоновых горутин + drain-контекст для in-flight + `defer amqpConn.Close()`: корректный graceful shutdown без потери/дублей.
3. **C4** — убрать удержание мьютекса через сон в `pkg/amqp.Channel`.
4. **C5 + C6** — отменяемый backoff и проброс `ctx` в публикацию (мелкие, но повсеместные).
5. **C7, C8** — singleflight/eviction в кэше и осмысленный prefetch (по мере роста нагрузки).

> Кросс-ссылки: C2/C3 пересекаются с областью БД (отсутствие транзакции в `incident.handleFiring`) и областью надёжности шины (стратегия Nack/requeue, DLQ) — детально в соответствующих разделах. C1 пересекается с областью наблюдаемости (readiness-проба не отражает состояние консьюмера).
