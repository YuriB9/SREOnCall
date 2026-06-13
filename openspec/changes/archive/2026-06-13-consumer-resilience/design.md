# Design: consumer-resilience

## Контекст

Все находки CH07 (C1–C6, C8, E2, F7) сводятся к одной причине: **нет единого корректного
каркаса жизненного цикла консьюмера**. Сейчас цикл `Channel → Qos → Consume → for/select`
скопирован в трёх сервисах, не переживает реконнект (C1), не имеет recover (E2), не
дренируется на shutdown (C2/C3), а `pkg/amqp.Connection`/`Publish` держат мьютекс через сон
(C4) и игнорируют `ctx` (C5/C6). Решение — вынести каркас в `pkg/amqp.Consume` и исправить
`Connection`/`Publisher`, затем переразвести `main` всех сервисов.

## Решение

### 1. `pkg/amqp.Consume` — единый каркас (F7, C1, E2, C8, C3, C5)

```go
// Handler обрабатывает один распарсенный конверт. nil → Ack.
// Ошибка → Nack с requeue. Ошибка, обёрнутая Drop(), → Nack без requeue (poison).
type Handler func(ctx context.Context, env Envelope) error

type ConsumeOptions struct {
    Queue          string
    Prefetch       int            // 0 → = Concurrency
    Concurrency    int            // 0 → 1 (строго последовательно, сохраняет порядок)
    HandlerTimeout time.Duration  // 0 → 30s; drain-таймаут на одно сообщение
    Logger         *slog.Logger
}

func Consume(ctx context.Context, conn *Connection, opts ConsumeOptions, h Handler) error
```

- **Supervisor-петля (C1).** `Consume` крутит `for ctx.Err() == nil { runOnce(...) }`. `runOnce`
  открывает канал, ставит `Qos`, `Consume`, обрабатывает доставки до закрытия `msgs` (реконнект
  брокера) или отмены `ctx`. При выходе с ошибкой и живом `ctx` — отменяемый экспоненциальный
  backoff (1→2→4→…→cap 30s), затем новая итерация. При `ctx.Done` — выход с `nil`.
- **Worker-pool (C8).** Внутри `runOnce` — `errgroup` с `SetLimit(concurrency)`; `prefetch`
  выставляется в `Qos` равным `concurrency`. По умолчанию `concurrency = 1` → строго
  последовательная обработка (сохраняет порядок `incident.created` перед `incident.updated`,
  критичный для escalation), но prefetch=1 не «придерживает» лишние сообщения. Сервис может
  поднять `Concurrency`, если порядок для него не важен.
- **`recover` на сообщение (E2).** Обработка каждой доставки обёрнута в `defer recover()`:
  паника логируется со стеком и конвертируется в `Nack(false, false)` (drop, без requeue —
  иначе crash-loop). Изолирует «отравленное» сообщение, не роняя процесс.
- **Drain-контекст (C3).** На каждое сообщение создаётся
  `ctx, cancel := context.WithTimeout(context.WithoutCancel(runCtx), HandlerTimeout)`.
  Обработка in-flight сообщения **не отменяется** при остановке сервиса, а доводится до конца
  (или до своего таймаута) — устраняет частичные записи incident. Вместе с graceful-drain в
  `main` (см. п.3) даёт корректный at-least-once без обрыва на полуслове.
- **Парсинг конверта (F7).** `runOnce` сам делает `json.Unmarshal(body, &env)`; невалидный
  конверт → лог + `Nack(false, false)` (drop). Handler получает готовый `Envelope` и
  декодирует payload из `env.Payload` (helper `DecodePayload(env, dst)`), устраняя двойной
  разбор в notification.

`Drop(err) error` оборачивает ошибку sentinel'ом `ErrDrop`; `Consume` через `errors.Is`
выбирает requeue (по умолчанию) либо drop. Это сохраняет различие политик по сервисам без
зашивания их в каркас.

### 2. `pkg/amqp.Connection` и `Publisher` (C4, C5, C6)

- **`Channel(ctx)` не держит мьютекс через I/O (C4).** Под `mu` — только чтение/смена
  указателя `conn`. Если соединение живо — сразу `conn.Channel()`. Иначе реконнект выполняется
  в `reconnect(ctx)`, сериализованном отдельным `reconnectMu` с double-check (повторно
  проверяем `conn` после захвата — кто-то мог уже переподключиться). Основной `mu` при этом
  свободен: остальные паблишеры либо быстро получают живой канал, либо ждут на `reconnectMu`,
  но не на залоченном на 31 c `mu`.
- **Отменяемый backoff (C5).** `dialWithRetry(ctx, attempts)` заменяет `time.Sleep(delay)` на
  `select { case <-ctx.Done(): return ctx.Err(); case <-time.After(delay): }`.
- **`Publish` пробрасывает `ctx` (C6).** `publish(ctx, …)` → `ch.PublishWithContext(ctx, …)`
  вместо `context.Background()`. Внешний ретрай-цикл `Publish` уже отменяем — приводим к
  единому образцу.

Смена сигнатуры `Channel() → Channel(ctx)` — внутренняя для монорепо; callers (publisher,
объявление топологии в каждом `main`) обновляются. Wire-формат не затрагивается.

### 3. Разводка в `main` (C2)

Во всех сервисах с фоновыми горутинами (incident: консьюмер; escalation: консьюмер + монитор;
notification: консьюмер) фоновые горутины собираются в `errgroup`, который дожидается их
**после** `srv.Shutdown`, затем `amqpConn` закрывается явно:

```go
g, gctx := errgroup.WithContext(ctx)
g.Go(func() error { return pkgamqp.Consume(gctx, amqpConn, opts, handler) })
g.Go(func() error { mon.Run(gctx); return nil })   // только escalation
...
<-ctx.Done()
srv.Shutdown(shutCtx)
_ = g.Wait()             // дренаж in-flight (drain-контекст доводит обработку)
_ = amqpConn.Close()     // явное закрытие соединения
```

`monitor.step` дополнительно получает `recover` на одну единицу работы (E2 для монитора).

### 4. Диспетчеры notification (C5)

`Mattermost.Send` и `Email.Send` заменяют `time.Sleep(backoff)` на отменяемое ожидание по
переданному `ctx`; `Email.Send` начинает использовать `ctx` (сейчас `_ context.Context`).
HTTP-запрос Mattermost уже использует `ctx` — правится только бэкофф.

## Совместимость и миграция

- **Wire-формат `Envelope`/payload не меняется** — сообщения, лежащие в очередях, и сообщения
  от старых версий читаются как прежде. Не BREAKING для очередей/консьюмеров.
- **Деплой:** новые и старые версии сервисов сосуществуют — формат сообщений идентичен.
  Порядок выката не важен. Откат безопасен (формат не менялся).
- **Семантика подтверждений сохранена:** incident/escalation requeue'ят ошибки обработки,
  drop'ают невалидный конверт; notification drop'ает любую ошибку обработки. Стратегия
  requeue/DLQ как таковая — вне объёма (отдельная находка области шины надёжности).
- **Поведение `/readyz` не меняется** в этом чейндже (остаётся статическим). Сигнал живости
  консьюмера для content-aware `/readyz` — CH10.

## Отклонённые альтернативы

- **Оставить supervisor-петлю в каждом `main` (без каркаса).** Отклонено: тиражирует C1-фикс
  по трём `main`, не решает F7 (копипаста цикла) и не централизует recover/drain — расходится
  при правках.
- **Worker-pool по умолчанию `Concurrency > 1`.** Отклонено: параллельная обработка ломает
  порядок `incident.created`/`incident.updated`, от которого зависит escalation (двойная
  эскалация / гонка stop). Дефолт = 1 (последовательно), параллелизм — осознанный opt-in.
- **`singleflight` для реконнекта `Connection`.** Отклонено в пользу `reconnectMu` + double-check:
  меньше зависимостей, та же гарантия «один дозвон за раз», проще читать.
- **Drop «отравленного» сообщения в DLQ вместо `Nack(false,false)`.** Отклонено как вне объёма:
  DLQ — отдельная находка области надёжности шины; здесь достаточно изолировать панику от
  процесса. Drop без requeue уже предотвращает crash-loop.
