# ADR-0017: Общий HTTP-bootstrap и content-aware пробы готовности

- Status: Accepted
- Date: 2026-06-14
- Change: shared-httpserver-and-readiness
- Affected: pkg/httpserver, pkg/amqp, pkg/auth, services/ingestion, services/incident, services/escalation, services/notification, services/scheduling

## Context

Bootstrap HTTP-сервера и middleware дублировался ~на 100 строк в каждом из пяти
`cmd/server/main.go` и уже разошёлся (аудит `docs/audit/01-structure-layers.md`,
`04-error-handling.md`, `07-observability.md`, `05-security.md`):

- **F4 (major):** таймауты несогласованы — incident/ingestion задают полный набор
  (`Read/Write/Idle`), escalation/notification — только `ReadHeaderTimeout`; порядок
  запуска/шатдауна сервера различается.
- **E1/O6 (major/minor):** `Recoverer` и `RequestID` подключены лишь в ingestion/incident;
  в остальных паника в хендлере рвёт соединение без `500` и без метрики.
- **O1 (major):** `/healthz` и `/readyz` во всех пяти — статический `200`. Readiness не
  отражает доступность Postgres/Redis/RabbitMQ и **не замечает тихую смерть консьюмера**
  (связка с C1, [ADR-0015](0015-consumer-lifecycle-and-resilience.md)): под остаётся «ready»,
  пока конвейер стоит.
- **F10/F5/F9 (minor):** fail-closed toggle auth ([ADR-0012](0012-fail-closed-auth-and-strict-jwt.md))
  скопирован в 4 `main`; scheduling строит логгер мимо `pkg/logger` без уровня; escalation
  дважды создаёт `escalator.New`.
- **E6 (minor):** incident отдаёт `err.Error()` в HTTP-ответ.
- **S6 (low):** входные webhook-эндпоинты не ограничены по частоте.

Общий bootstrap всех сервисов и семантика проб готовности, влияющая на поведение
Kubernetes, — значимое кросс-каттинг-решение, поэтому фиксируется ADR.

## Options considered

- **Единый пакет `pkg/httpserver` (Run + NewRouter + ReadyHandler + RateLimit) и сигнал
  живости консьюмера в `pkg/amqp`.** Централизует таймауты, обязательный middleware, пробы и
  rate-limit в одной не-забываемой точке; сервис задаёт только состав readiness-проверок.
  Принято.
- **Вынести только константы таймаутов, сервер оставить в `main`.** Отклонено: не устраняет
  расхождение middleware/порядка шатдауна и не даёт единой точки для `/readyz`.
- **Heartbeat по времени последней обработки как сигнал живости консьюмера.** Отклонено:
  на простаивающей очереди давал бы ложный «down»; достаточно признака «канал открыт и
  consume идёт».
- **Распределённый rate-limit на Redis (как у notification).** Отклонено для входа: per-pod
  in-memory достаточно против ресурсного флуда (S6 — Low), без новой зависимости на горячем пути.

## Decision

### `pkg/httpserver`

- **`Run(ctx, addr, handler, logger)` (F4):** единый `http.Server` с таймаутами
  `Read=ReadHeader=Write=15s`, `Idle=60s`; блокирует до отмены `ctx`, затем graceful
  `Shutdown` (10s). `main` решает про `os.Exit` по возвращённой ошибке.
- **`NewRouter(service, checks...)` (E1/O6/O1):** chi-роутер с обязательной цепочкой
  `RequestID → Recoverer → metrics` и стандартными эндпоинтами — статический `/healthz`,
  content-aware `/readyz`, `/metrics`. Забыть recovery/request-id в новом сервисе нельзя.
- **`/readyz` (O1):** реестр именованных проверок `Check{Name, Probe func(ctx) error}` под
  общим таймаутом 3s; любой сбой → `503`, иначе `200`. Тело отдаёт по каждой проверке
  `ok`/`down` — **деталь ошибки наружу не уходит**, только HTTP-статус (согласуется с E6).
  `/healthz` остаётся лёгким liveness без внешних зависимостей: временный сбой БД снимает под
  с балансировки (readiness), но не перезапускает его.
- **`RateLimit(rps, burst)` (S6):** per-IP token bucket (`golang.org/x/time/rate`) с ленивым
  вытеснением неактивных ключей; `429` при превышении. Применяется на webhook-роутах ingestion.

### Сигнал живости консьюмера — `amqp.Probe` (O1)

`pkg/amqp.Probe` (atomic Up/Down) выставляется supervisor-петлёй `Consume`: Up, когда канал
открыт и consume идёт; Down при разрыве/реконнекте/остановке. `Connection.Ready()` отражает
открытость соединения. `/readyz` читает `Probe.Healthy()` через `httpserver.BoolCheck` —
закрывает наблюдательную половину C1 (точка, оставленная открытой в ADR-0015).

### Точечные решения

- **F10:** `auth.MiddlewareOrPassthrough(opts, authDisabled, logger)` — единственная реализация
  fail-closed toggle; поведение ADR-0012 сохранено 1:1.
- **F5:** scheduling переходит на `pkglogger.New(cfg.LogLevel)` (добавлено поле `LogLevel`).
- **F9:** escalation создаёт `escalator.New` один раз и передаёт в consumer/handler/monitor.
- **E6:** incident возвращает стабильное доменное сообщение (через `errors.As`
  `ErrInvalidTransition`), технические детали — только в лог.

Контракты API, события RabbitMQ и схема БД **не меняются** — решение про bootstrap и
семантику проб, не про контракты (не BREAKING).

## Consequences

- `/readyz` теперь возвращает `503` при реальной недоступности Postgres/Redis/RabbitMQ или
  при отвалившемся консьюмере → Kubernetes снимает под с трафика. Это закрывает «слепое пятно»
  вокруг тихой смерти консьюмера; `/healthz` остаётся статическим, под не убивается из-за
  временного сбоя зависимости. Манифесты проб менять не требуется.
- `pkg/httpserver` — единственная точка эволюции HTTP-bootstrap; новые сервисы используют
  `Run`/`NewRouter`, не копируют сервер и middleware.
- Все пять сервисов получают одинаковые таймауты, `Recoverer`, `RequestID` и пробы.
- `Probe` отражает подключение консьюмера к брокеру, но не «завис в обработке одного
  сообщения» — точные метрики обработки относятся к CH11 (O2). Per-message timeout уже есть
  в `Consume` (ADR-0015).
- Rate-limit — per-pod (in-memory); защищает от ресурсного флуда входа, не делится между
  репликами. Распределённый лимит — отдельной находкой при необходимости.
- Открывает CH11 (метрик-middleware и доменные метрики живут поверх `pkg/httpserver`),
  CH12 (`request_id` из общего middleware), CH13 (`otelhttp` в общую цепочку).
