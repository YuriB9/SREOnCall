## Context

Каждый из пяти `cmd/server/main.go` повторяет ~100 строк bootstrap'а HTTP-сервера и
middleware. Копипаста уже разошлась (F4): incident/ingestion задают полный набор
таймаутов (`ReadTimeout/WriteTimeout/IdleTimeout`), escalation/notification —
только `ReadHeaderTimeout`; `Recoverer` (E1) и `RequestID` (O6) подключены лишь в
ingestion/incident. Пробы `/healthz` и `/readyz` во всех пяти сервисах — статический
`200` (O1): readiness не отражает доступность Postgres/Redis/RabbitMQ и не замечает
тихую смерть консьюмера (связка с C1 из CH07). Дополнительно: scheduling строит
логгер мимо `pkg/logger` без уровня (F5), escalation дважды создаёт `escalator.New`
(F9), fail-closed toggle auth дословно скопирован в 4 `main.go` (F10), incident отдаёт
`err.Error()` в HTTP-ответ (E6), входные эндпоинты не ограничены по частоте (S6).

CH07 уже переработал `pkg/amqp` (supervisor-реконнект, `Consume`-каркас) и завёл
graceful-drain через `errgroup` в `main`. CH03 свёл auth к `pkgauth.Middleware(Options)`
с fail-closed-политикой. CH10 консолидирует оставшийся bootstrap в общий пакет.

## Goals / Non-Goals

**Goals:**
- Единая точка bootstrap'а HTTP (`pkg/httpserver`): таймауты, graceful shutdown,
  обязательный middleware-набор, health/ready/metrics-эндпоинты — F4, E1, O6.
- Content-aware `/readyz`, отражающий доступность критичных зависимостей и живость
  консьюмера; `503` при сбое — O1.
- Переиспользуемый входной rate-limit для webhook-эндпоинтов ingestion — S6.
- Вынести auth-toggle в `pkg/auth` — F10; выровнять scheduling-логгер — F5;
  один `escalator.New` — F9; убрать `err.Error()` из ответа incident — E6.

**Non-Goals:**
- Доменные/шинные метрики и alert-правила (O2/O5/R1) — это CH11.
- Распределённая трассировка и проброс trace-context (O3) — CH13.
- Изменение wire-формата событий, схемы БД, контрактов API.
- DLQ и прочая надёжность шины — вне объёма.

## Decisions

### 1. `pkg/httpserver.Run` вместо копий `http.Server` (F4)
`Run(ctx, addr, handler, logger) error` строит `http.Server` с едиными таймаутами
(`ReadTimeout=15s`, `ReadHeaderTimeout=15s`, `WriteTimeout=15s`, `IdleTimeout=60s`),
запускает `ListenAndServe` и по `ctx.Done()` делает `Shutdown` с таймаутом 10s.
Возвращает ошибку (кроме `ErrServerClosed`) — `main` сам решает про `os.Exit`.
*Альтернатива* — оставить сервер в `main`, вынести только таймауты-константы: отвергнута,
не устраняет расхождение порядка запуска/шатдауна.

### 2. Базовый middleware-набор как обязательный (E1, O6)
`httpserver.BaseMiddleware(service)` возвращает цепочку `RequestID → Recoverer →
metrics.Middleware(service)` в фиксированном порядке. Применяется helper'ом
`httpserver.NewRouter(service)`, который сразу регистрирует `/healthz`, `/metrics` и
готов принять readiness-проверки. Это делает невозможным «забыть» recovery/request-id
при заведении сервиса.

### 3. Content-aware `/readyz` через реестр проверок (O1)
`type Check struct { Name string; Probe func(ctx) error }`. `/readyz` исполняет все
проверки с общим таймаутом (3s), при первой/любой ошибке отвечает `503` и JSON
`{"status":"unavailable","checks":{...}}`; иначе `200`. `/healthz` остаётся статическим
liveness (под не должен перезапускаться из-за временно недоступной БД — только сниматься
с балансировки). Состав проверок задаёт `main` каждого сервиса:
- ingestion: Postgres `pool.Ping`, Redis `rdb.Ping`, AMQP `conn.Ready`.
- incident: Postgres, AMQP `conn.Ready`, консьюмер `cons.Healthy`.
- escalation/notification: Postgres (+ Redis у notification), и AMQP-probe **только если**
  `RABBITMQ_URL` задан (консьюмер опционален).
- scheduling: Postgres (+ Redis, если клиент поднялся).

### 4. Сигнал живости консьюмера — `Probe` в `pkg/amqp` (O1)
Добавляется `amqp.Probe` с `atomic.Bool`. `ConsumeOptions.Probe *Probe`: `consumeOnce`
выставляет `Up` после успешного `Consume` и `Down` при выходе (broker drop/реконнект),
supervisor держит `Down` между ретраями. `Connection.Ready() bool` = `current() != nil`.
`/readyz` читает `Probe.Healthy()` — отражает «консьюмер подключён к брокеру». Это
закрывает явный остаток из заметки CH07 («сигнал состояния консьюмера завести вокруг
`pkg/amqp.Consume`»). *Альтернатива* — heartbeat по времени последней обработки:
отвергнута как избыточная (на простаивающей очереди давала бы ложные `Down`).

### 5. Auth-toggle в `pkg/auth` (F10)
`MiddlewareOrPassthrough(opts Options, authDisabled bool, logger *slog.Logger)
(func(http.Handler) http.Handler, error)`: при `opts.JWKSURL != ""` строит
`Middleware` (с warn про незаданные iss/aud), при `authDisabled` — passthrough с warn,
иначе — ошибка `ErrAuthNotConfigured` (fail-closed; `main` логирует и `os.Exit(1)`).
Поведение CH03 сохраняется 1:1, дублирование из 4 `main.go` убирается.

### 6. Rate-limit middleware (S6)
`httpserver.RateLimit(rps, burst)` — per-client (по IP из `RemoteAddr`) token bucket на
`golang.org/x/time/rate`, с фоновым вытеснением неактивных ключей (TTL), чтобы карта не
росла неограниченно. Применяется на webhook-роутах ingestion. In-memory (per-pod) —
достаточно против ресурсного флуда; распределённый лимит на Redis — вне объёма (Low-риск
в аудите). *Альтернатива* — Redis token bucket как в notification: отвергнута как
избыточная для per-pod защиты входа.

### 7. E6 — стабильное доменное сообщение
В `incident/handler.go` `PatchStatus`: вместо `http.Error(w, err.Error(), 422)` —
`errors.As(err, &statemachine.ErrInvalidTransition)` → отдать стабильное сообщение
(текст типизированной ошибки безопасен), прочие ошибки → `"internal error"` + лог.

## Risks / Trade-offs

- **`/readyz` начинает отдавать 503** при недоступности зависимостей → k8s снимет под с
  трафика. Это и есть цель O1. *Mitigation:* `/healthz` (liveness) остаётся статическим,
  под не убивается из-за временного сбоя БД; readiness-таймаут 3s не даёт пробе виснуть.
- **Probe как proxy живости консьюмера** не ловит «завис в обработке одного сообщения»
  (только разрыв соединения). *Mitigation:* приемлемо для CH10; точные метрики обработки —
  O2/CH11. Per-message timeout уже есть в `Consume` (CH07).
- **In-memory rate-limit не делится между подами** — лимит на под, не на кластер.
  *Mitigation:* достаточно против ресурсного исчерпания (Low в аудите); распределённый —
  отдельной находкой при необходимости.
- **Широкая переразводка 5 `main.go`** → риск регрессий wiring. *Mitigation:* `go build/
  vet/test` всех модулей, `-race` (трогается `pkg/amqp`), ручная проверка маршрутов.

## Migration Plan

- Чисто аддитивно на уровне контрактов: API/события/схема БД не меняются, миграций нет.
- Деплой по сервисам в любом порядке (общие пакеты собираются в каждый бинарь статически).
- Откат — возврат образа: статический `/readyz` вернётся, остальное поведение совместимо.
- Операторам: после раскатки `/readyz` может отдавать `503` при реальной недоступности
  зависимостей — это корректное поведение, а не регресс. Манифесты проб менять не нужно.

## Open Questions

Нет.
