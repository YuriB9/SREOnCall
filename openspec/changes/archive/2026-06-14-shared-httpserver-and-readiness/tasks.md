# Задачи: shared-httpserver-and-readiness (CH10)

Каждая задача привязана к находке аудита и `file:line`. Объём — строго находки CH10
(F4, F5, F9, F10, E1, E6, O1, O6, S6); соседние находки не трогаем.

## 1. pkg/httpserver (новый пакет) — F4, E1, O6, O1, S6

- [x] 1.1 F4 — `Run(ctx, addr, handler, logger)`: единый `http.Server` с таймаутами
  (Read/ReadHeader/Write=15s, Idle=60s) + graceful shutdown (10s). Источник дублей:
  `services/incident/cmd/server/main.go:137-159`, `escalation:157-173`,
  `notification:151-167`, `scheduling:181-194`, `ingestion:102-121`.
- [x] 1.2 E1+O6 — `BaseMiddleware(service)` = `RequestID → Recoverer → metrics`;
  `NewRouter(service)` регистрирует базовый набор + `/healthz` + `/metrics`. Источник:
  нет `Recoverer`/`RequestID` в `escalation/main.go:127`, `notification/main.go:134`,
  `scheduling/main.go:129` (`docs/audit/04-error-handling.md` E1, `07-observability.md` O6).
- [x] 1.3 O1 — реестр `Check{Name, Probe}` и `ReadyHandler(checks)`: `/readyz` гоняет
  проверки с таймаутом, `503`+JSON при сбое; `/healthz` остаётся статическим liveness.
  Источник: `incident/main.go:108-109` и идентично в 4 остальных (`07-observability.md` O1).
- [x] 1.4 S6 — `RateLimit(rps, burst)` middleware: per-IP token bucket
  (`golang.org/x/time/rate`) + вытеснение неактивных ключей. Источник:
  `docs/audit/05-security.md` S6.
- [x] 1.5 Package-doc и юнит-тесты: таймауты `Run`, цепочка middleware, `/readyz`
  200/503, rate-limit (отказ при превышении).

## 2. pkg/amqp — O1 (сигнал живости консьюмера)

- [x] 2.1 O1 — `Probe` (atomic Up/Down) + `Healthy()`; поле `ConsumeOptions.Probe`;
  выставление Up/Down в `consumeOnce`/supervisor. `pkg/amqp/consume.go:82-149`.
- [x] 2.2 O1 — `Connection.Ready() bool` (= `current() != nil`). `pkg/amqp/amqp.go:44-52`.
- [x] 2.3 Юнит-тест `Probe` (round-trip Up/Down).

## 3. pkg/auth — F10 (вынос auth-toggle)

- [x] 3.1 F10 — `MiddlewareOrPassthrough(opts, authDisabled, logger)`: Middleware /
  passthrough / `ErrAuthNotConfigured` (fail-closed), warn про iss/aud. Источник дублей:
  `incident/main.go:76-100`, `escalation:98-122`, `notification:105-129`,
  `scheduling:91-115` (`docs/audit/01-structure-layers.md` F10).
- [x] 3.2 Юнит-тест трёх веток (jwks/disabled/неконфигурировано).

## 4. services/incident — E6 + переразводка main

- [x] 4.1 E6 — `internal/handler/handler.go:174`: `err.Error()` → `errors.As`
  `statemachine.ErrInvalidTransition` (стабильное сообщение), прочее → `"internal error"`
  + лог. (`docs/audit/04-error-handling.md` E6).
- [x] 4.2 Переразводка `cmd/server/main.go` на `pkg/httpserver` (Run, NewRouter, auth-toggle)
  + content-aware `/readyz` (Postgres + AMQP `conn.Ready` + `cons.Healthy`).

## 5. services/escalation — F9 + переразводка main

- [x] 5.1 F9 — один `escalator.New` до ветвления AMQP, передать в consumer/handler/monitor.
  `cmd/server/main.go:81` и `:86` (`docs/audit/01-structure-layers.md` F9).
- [x] 5.2 Переразводка `main.go` на `pkg/httpserver` + `/readyz` (Postgres + AMQP-probe
  только при заданном `RABBITMQ_URL`).

## 6. services/notification — переразводка main

- [x] 6.1 Переразводка `cmd/server/main.go` на `pkg/httpserver` + `/readyz`
  (Postgres + Redis + AMQP-probe при заданном `RABBITMQ_URL`).

## 7. services/scheduling — F5 + переразводка main

- [x] 7.1 F5 — добавить `LogLevel` в `internal/config/config.go`, перейти на
  `pkglogger.New(cfg.LogLevel)` вместо `slog.New(...)` без уровня. `cmd/server/main.go:26`
  (`docs/audit/01-structure-layers.md` F5).
- [x] 7.2 Переразводка `main.go` на `pkg/httpserver` + `/readyz` (Postgres + Redis, если
  клиент поднялся). Сохранить `upsertUserMW`.

## 8. services/ingestion — S6 + переразводка main

- [x] 8.1 S6 — применить `httpserver.RateLimit` на webhook-роутах
  (`cmd/server/main.go:95-99`).
- [x] 8.2 Переразводка `main.go` на `pkg/httpserver` + `/readyz`
  (Postgres + Redis + AMQP `conn.Ready`).

## 9. ADR

- [x] 9.1 `docs/adr/0017-shared-httpserver-and-readiness.md` — общий bootstrap + семантика
  content-aware readiness + probe живости консьюмера.

## 10. Верификация

- [x] 10.1 `go build ./...`, `go vet ./...`, `go test ./...` во всех 6 модулях.
- [x] 10.2 `go test -race ./...` (затронут `pkg/amqp`).
- [x] 10.3 `golangci-lint run --new-from-merge-base main` помодульно (0 new issues).
- [x] 10.4 `govulncheck ./...` помодульно (0 достижимых).
- [x] 10.5 `go mod tidy` в задетых модулях — без диффа (учесть `golang.org/x/time`).
- [x] 10.6 `/opsx:verify` → `/opsx:archive --skip-specs` (no-delta) → обновить статус CH10
  в `docs/audit/00-roadmap.md` (дашборд + строка). `docs/spec-vs-code-audit.md` — обновить,
  если закрывает пункты сверки.
