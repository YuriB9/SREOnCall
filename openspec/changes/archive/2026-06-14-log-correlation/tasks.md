## 1. pkg/logger — O4: context-aware slog-хендлер

- [x] 1.1 O4 — добавить приватный `contextHandler`, оборачивающий базовый `slog.Handler`: в `Handle(ctx, r)` достаёт `request_id` через `chiMiddleware.GetReqID(ctx)` и при непустом значении добавляет `r.AddAttrs(slog.String("request_id", id))`; делегирует `Enabled/WithAttrs/WithGroup`. Точка расширения под `trace_id`/`span_id` (CH13) — комментарием. `pkg/logger/logger.go`
- [x] 1.2 O4 — `New(level)` оборачивает `slog.NewJSONHandler(...)` в `contextHandler` перед `slog.New`; глобальный default — тоже обёрнутый. `pkg/logger/logger.go:23`
- [x] 1.3 O4 — юнит-тест: лог через `*Context` с `request_id` в ctx → запись содержит поле `request_id`; без ключа в ctx → поля нет; не-Context-вызов → поля нет. `pkg/logger/logger_test.go` (новый)

## 2. HTTP-хендлеры — O4: перевод логов на *Context (request_id из r.Context())

- [x] 2.1 O4 — `h.logger.Error/Warn(...)` → `*Context(r.Context(), ...)` во всех вызовах. `services/scheduling/internal/handler/handler.go`
- [x] 2.2 O4 — то же. `services/incident/internal/handler/handler.go`
- [x] 2.3 O4 — то же. `services/escalation/internal/handler/handler.go`
- [x] 2.4 O4 — то же. `services/notification/internal/handler/handler.go`
- [x] 2.5 O4 — то же. `services/ingestion/internal/handler/handler.go`

## 3. Фоновые пути — O4: перевод логов на *Context (задел под trace_id, CH13)

- [x] 3.1 O4 — `internal/notifier`: логи на `*Context(ctx, ...)` (ctx уже в сигнатурах). `services/notification/internal/notifier/notifier.go`
- [x] 3.2 O4 — `internal/escalator`: `e.logger.*` → `*Context(ctx, ...)`. `services/escalation/internal/escalator/escalator.go`
- [x] 3.3 O4 — `internal/monitor`: `m.logger.*` → `*Context(ctx, ...)`. `services/escalation/internal/monitor/monitor.go`
- [x] 3.4 O4 — консьюмеры incident/escalation/notification: `c.logger.*` → `*Context(ctx, ...)`. `services/{incident,escalation,notification}/internal/consumer/consumer.go`
- [x] 3.5 O4 — `pkg/amqp`: `opts.Logger.*` и `slog.Warn` в consume/publish-путях → `*Context` с доступным `ctx`/`runCtx`. `pkg/amqp/consume.go:97,138,169,178,191,196`, `pkg/amqp/amqp.go:96,144`

## 4. E5 — закрепление единого ключа "err" линтером

- [x] 4.1 E5 — добавить `sloglint` в `linters.enable` и `linters.settings.sloglint.forbidden-keys: ["error"]` (ключ ошибки в slog только `"err"`). `.golangci.yml`
- [x] 4.2 E5 — проверить, что в продовом коде 0 slog-вызовов с ключом `"error"` (JSON-тела `{"error": ...}` — контракт API, не slog, не трогаем). grep-проверка перед прогоном линтера.

## 5. Верификация

- [x] 5.1 `go build ./...` + `go vet ./...` всех 6 модулей (помодульно, `GOWORK=off` для сервисов).
- [x] 5.2 `go test ./...` всех затронутых модулей; `-race` для `pkg/logger` и `pkg/amqp`.
- [x] 5.3 `golangci-lint run --new-from-merge-base main` помодульно (pkg — без `GOWORK=off`; сервисы — с `GOWORK=off`): 0 new issues, sloglint не ругается.
- [x] 5.4 `govulncheck ./...` помодульно — 0 достижимых.
- [x] 5.5 `go mod tidy` без диффа во всех модулях.
- [x] 5.6 Обновить статус CH12 в `docs/audit/00-roadmap.md` (дашборд + строка чейнджа с заметкой для следующих сессий).
