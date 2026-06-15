# Tasks — bus-publish-perf (CH14, находка P1)

Объём строго = находка **P1** из «Матрицы покрытия» (`docs/audit/00-roadmap.md`).
Источник истины: [docs/audit/10-performance.md §P1](../../../docs/audit/10-performance.md).

## 1. pkg/amqp — переиспользуемый канал публикации (P1)

- [x] 1.1 **P1** — добавить `mu sync.Mutex` и `ch *amqp.Channel` в `Publisher`, `pkg/amqp/amqp.go` ([amqp.go:117](../../../pkg/amqp/amqp.go#L117)).
- [x] 1.2 **P1** — приватный `channel(ctx)` (под `mu`): вернуть кешированный канал, если `ch != nil && !ch.IsClosed()`, иначе открыть через `conn.Channel(ctx)` и закешировать, `pkg/amqp/amqp.go`.
- [x] 1.3 **P1** — `resetChannel()` (под `mu`): закрыть и обнулить `ch`, `pkg/amqp/amqp.go`.
- [x] 1.4 **P1** — переписать `publish(ctx, …)` ([amqp.go:157](../../../pkg/amqp/amqp.go#L157)): захватить `mu`, получить канал через `channel(ctx)`, `PublishWithContext`; при ошибке — `resetChannel()` и вернуть ошибку (ретрай в `publishWithRetry` переоткроет). Убрать `conn.Channel()`+`defer ch.Close()` на сообщение.
- [x] 1.5 **P1** — `Publisher.Close() error`: закрыть кешированный канал под `mu`, `pkg/amqp/amqp.go`.

## 2. Разводка остановки в сервисах-издателях

- [x] 2.1 incident: вызвать `pub`-`Publisher.Close()` при graceful-shutdown перед `amqpConn.Close()`, `services/incident/cmd/server/main.go` ([main.go:67,121](../../../services/incident/cmd/server/main.go#L67)).
- [x] 2.2 escalation: то же, `services/escalation/cmd/server/main.go` ([main.go:80,157](../../../services/escalation/cmd/server/main.go#L80)).
- [x] 2.3 ingestion: то же, `services/ingestion/cmd/server/main.go` ([main.go:75](../../../services/ingestion/cmd/server/main.go#L75)).

> Примечание: `publisher.New(...)` оборачивает `pkgamqp.Publisher`. Если обёртка не
> экспонирует `Close()`, прокинуть его (или закрывать `pkgamqp.Publisher` напрямую из `main`).

## 3. Бенчмарк (benchstat до/после в коммите)

- [x] 3.1 **P1** — `BenchmarkPublish` в `pkg/amqp/amqp_bench_test.go`: подключение к `RABBITMQ_URL` (по умолчанию `amqp://oncall:oncall@localhost:5672/` из docker-compose), `b.Skip` при недоступности брокера; публиковать в тестовый exchange.
- [x] 3.2 Снять benchstat **до** (на старом коде) и **после** на локальном docker-compose RabbitMQ; приложить вывод к сообщению коммита.

## 4. Верификация (Definition of Done, `CHANGE-KICKOFF.md` §3)

- [x] 4.1 `go build ./...`, `go vet ./...`, `go test ./...` — все 6 модулей (pkg-сигнатура `Publisher` тронута → собрать все).
- [x] 4.2 `go test -race ./...` в `pkg/amqp` (конкурентный доступ к каналу).
- [x] 4.3 `golangci-lint run --new-from-merge-base main` (pkg — без `GOWORK=off`; задетые сервисные модули — с `GOWORK=off`).
- [x] 4.4 `govulncheck ./...` — чисто.
- [x] 4.5 `go mod tidy` в задетых модулях — без диффа.
- [ ] 4.6 `/opsx:verify`.

## 5. Хэндофф

- [ ] 5.1 `/opsx:archive` (с `--skip-specs` — no-delta).
- [ ] 5.2 Обновить `docs/audit/00-roadmap.md`: CH14 → `✅ done` в дашборде и в строке чейнджа (заметка для CH15: издатель теперь на переиспользуемом канале).
- [x] 5.3 `docs/spec-vs-code-audit.md` — обновлять не требуется (поведение/спек не меняются).
