# Tasks: consumer-resilience

Каждая задача привязана к находке аудита (`docs/audit/02-concurrency.md`,
`04-error-handling.md`, `01-structure-layers.md`) и месту в коде.

## 1. pkg/amqp — Connection и Publisher

- [x] 1.1 C4 — `Channel()` → `Channel(ctx)`; не держать `mu` через дозвон/сон: под `mu`
  только указатель `conn`, реконнект в `reconnect(ctx)` под отдельным `reconnectMu` с
  double-check. `pkg/amqp/amqp.go:39-48` (`Channel`), `:50-61` (`dialWithRetry`).
- [x] 1.2 C5 — `dialWithRetry(ctx, attempts)`: `time.Sleep(delay)` → отменяемое
  `select { <-ctx.Done(); <-time.After(delay) }`. `pkg/amqp/amqp.go:58`.
- [x] 1.3 C6 — `Publish`/`publish` пробрасывают `ctx` в `PublishWithContext(ctx, …)` вместо
  `context.Background()`. `pkg/amqp/amqp.go:84-93` (`Publish`), `:103-121` (`publish`,
  строка `:110`).
- [x] 1.4 Обновить callers `Channel()` → `Channel(ctx)`: объявление топологии в
  `services/{incident,escalation,notification}/cmd/server/main.go` (вызовы `amqpConn.Channel()`).

## 2. pkg/amqp — каркас Consume (F7, C1, E2, C8, C3, C5)

- [x] 2.1 F7 — новый файл `pkg/amqp/consume.go`: тип `Handler`, `ConsumeOptions`,
  `Consume(ctx, conn, opts, h)`; helper `DecodePayload(env, dst)`; `Drop(err)` + sentinel
  `ErrDrop`. Заменяет копипасту цикла из `*/consumer.go`.
- [x] 2.2 C1 — supervisor-петля: внешний `for ctx.Err()==nil`, переоткрытие канала/потока при
  закрытии `msgs`, отменяемый экспоненциальный backoff (cap 30s). Замена «горутина умирает» из
  `incident/consumer.go:71-73`, `escalation/consumer.go:46-47`, `notification/consumer.go:46-47`.
- [x] 2.3 C8 — `errgroup.SetLimit(Concurrency)`, `Qos(prefetch=Concurrency)`, дефолт
  `Concurrency=1`. Замена холостого `Qos(10,0,false)`: `incident/consumer.go:56`,
  `escalation/consumer.go:31`, `notification/consumer.go:31`.
- [x] 2.4 E2 — `defer recover()` вокруг обработки одной доставки → лог стека + `Nack(false,false)`.
  Закрывает отсутствие барьера в `incident/consumer.go:93`, `notification`/`escalation` handle.
- [x] 2.5 C3 — drain-контекст на сообщение: `WithTimeout(WithoutCancel(runCtx), HandlerTimeout)`
  (дефолт 30s). Закрывает обработку с отменённым `ctx`: `incident/consumer.go:75,106`.
- [x] 2.6 C5 — отменяемый backoff в supervisor-петле (тот же приём, что в `Publish`).

## 3. Консьюмеры сервисов — переход на каркас (F7)

- [x] 3.1 incident — `consumer.Run` → тонкий handler на `pkg/amqp.Consume`; сохранить семантику
  (ошибка обработки → requeue, невалидный конверт/паника → drop). `incident/internal/consumer/consumer.go:49-115`.
- [x] 3.2 escalation — то же; свести `handle` к маршрутизации по `env.Type` поверх каркаса,
  убрать дубль `switch` между `handle` и `ProcessDelivery`. `escalation/internal/consumer/consumer.go:24-115`.
- [x] 3.3 notification — то же; убрать двойной разбор конверта (`json.Unmarshal` + `Unwrap`),
  семантика «любая ошибка → drop» через `Drop()`. `notification/internal/consumer/consumer.go:24-89`.
- [x] 3.4 Сохранить экспортированные `ProcessDelivery` для интеграционных тестов (или
  адаптировать тесты под новую сигнатуру handler).

## 4. Разводка в main — graceful drain (C2)

- [x] 4.1 incident — фоновый консьюмер в `errgroup`; `g.Wait()` после `srv.Shutdown`;
  `defer amqpConn.Close()`. `services/incident/cmd/server/main.go:132-159`.
- [x] 4.2 escalation — консьюмер + монитор в `errgroup`; `g.Wait()` после `srv.Shutdown`;
  `defer amqpConn.Close()`. `services/escalation/cmd/server/main.go:80-92,163-167`.
- [x] 4.3 notification — консьюмер в `errgroup`; `g.Wait()` после `srv.Shutdown`;
  `defer amqpConn.Close()`. `services/notification/cmd/server/main.go:90-97,155-159`.

## 5. Монитор и диспетчеры

- [x] 5.1 E2 — `recover` на единицу работы в `monitor.step`. `services/escalation/internal/monitor/monitor.go:44-56`.
- [x] 5.2 C5 — Mattermost backoff: `time.Sleep` → отменяемое ожидание по `ctx`.
  `services/notification/internal/dispatcher/mattermost.go:55`.
- [x] 5.3 C5 — email backoff: `time.Sleep` → отменяемое ожидание; задействовать `ctx`
  (сейчас `_ context.Context`). `services/notification/internal/dispatcher/email.go:39,88`.

## 6. Тесты

- [x] 6.1 Юнит-тесты `pkg/amqp.Consume`: ack при успехе, requeue при ошибке, drop при `Drop()`/
  невалидном конверте, recover при панике (→ drop, без requeue). Без брокера (мок `Acknowledger`/
  фейковый источник доставок).
- [x] 6.2 Адаптировать существующие тесты консьюмеров к новой сигнатуре handler.
- [x] 6.3 Обновить `go mod tidy` в задетых модулях (`golang.org/x/sync` → direct).

## 7. Верификация

- [x] 7.1 `go build ./...` и `go vet ./...` во всех модулях (`pkg`, incident, escalation,
  notification, ingestion, scheduling — смена сигнатуры `pkg/amqp`).
- [x] 7.2 `go test ./...` во всех задетых модулях.
- [x] 7.3 `go test -race ./...` (конкурентность — обязательно для CH07).
- [x] 7.4 `golangci-lint run --new-from-merge-base main` — 0 новых замечаний.
- [x] 7.5 `govulncheck ./...` — чисто (новых зависимостей с CVE не вводим).
- [x] 7.6 `/opsx:verify` → `/opsx:archive`.
- [x] 7.7 Обновить статус CH07 в `docs/audit/00-roadmap.md` (дашборд + строка чейнджа).
