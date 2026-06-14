# Предложение: shared-httpserver-and-readiness (CH10)

## Зачем

Бутстрап HTTP-сервера дублируется ~на 100 строк в каждом `cmd/server/main.go`
пяти сервисов и уже разошёлся: у incident/ingestion заданы
`ReadTimeout/WriteTimeout/IdleTimeout`, а у escalation/notification — только
`ReadHeaderTimeout`; `Recoverer` и `RequestID` подключены лишь в ingestion и
incident; пробы `/healthz` и `/readyz` во всех пяти сервисах — статический `200`,
который не отражает состояние БД/Redis/RabbitMQ и не замечает тихую смерть
консьюмера. Это закрывает находки аудита **F4, F5, F9, F10, E1, E6, O1, O6, S6**
(`docs/audit/01-structure-layers.md`, `04-error-handling.md`, `05-security.md`,
`07-observability.md`).

## Что меняется

- **`pkg/httpserver` (новый):**
  - `Run(ctx, addr, handler, logger)` — единые таймауты и graceful shutdown (**F4**).
  - Общий middleware-набор `RequestID + Recoverer + metrics` одной точкой, чтобы
    его нельзя было «забыть» в новом сервисе (**E1, O6**).
  - Content-aware `/readyz`: реестр именованных проверок зависимостей, `503` при
    любом сбое; `/healthz` остаётся лёгким liveness без внешних зависимостей (**O1**).
  - Переиспользуемый `RateLimit` middleware (per-IP token bucket) для входных
    эндпоинтов (**S6**).
- **`pkg/amqp`:** лёгкий `Probe` (atomic up/down), который выставляет supervisor-петля
  `Consume`, и `Connection.Ready()` — сигнал «консьюмер жив и подключён» для `/readyz`
  (**O1**; закрывает остаток, отмеченный в заметке CH07).
- **`pkg/auth`:** `MiddlewareOrPassthrough(opts, authDisabled, logger)` — выносит
  дословно продублированный в 4 `main.go` fail-closed toggle auth-middleware (**F10**).
- **scheduling:** добавить `LogLevel` в конфиг и перейти на `pkglogger.New(cfg.LogLevel)`
  вместо ручного `slog.New(...)` без уровня (**F5**).
- **escalation:** создавать `escalator.New` один раз и передавать в consumer/handler/monitor
  (**F9**).
- **incident:** не отдавать `err.Error()` в HTTP-ответ — стабильное доменное сообщение,
  технические детали только в лог (**E6**).
- Переразводка всех пяти `cmd/server/main.go` на `pkg/httpserver` с
  content-aware `/readyz`.

## Impact

- **Затронутые сервисы:** ingestion, incident, escalation, notification, scheduling
  (переразводка `cmd/server/main.go`); общие пакеты `pkg/httpserver` (новый),
  `pkg/amqp`, `pkg/auth`.
- **События RabbitMQ:** не меняются. Wire-формат `pkg/amqp.Envelope` и payload'ы
  `pkg/events` не трогаются — сообщения в очередях читаются как прежде.
- **API:** контракты эндпоинтов не меняются. `/healthz` сохраняет статический `200`.
- **НЕ BREAKING** для API и событий. **Операционное изменение поведения:** `/readyz`
  теперь возвращает `503`, когда недоступна критичная зависимость (Postgres/Redis/AMQP)
  или консьюмер не подключён — Kubernetes пометит под `NotReady` и снимет с него трафик.
  Это и есть цель находки O1; манифесты `deploy/k8s` уже опрашивают `/readyz`, новых
  настроек проб не требуется.
- **Спецификации:** продуктовых capability в `openspec/specs/*` изменение не затрагивает
  (общий bootstrap, middleware, семантика проб и rate-limit — инфраструктура/операционка).
  Дельта спека не вводится (прецедент CH06/CH07/CH09). Значимое решение по семантике
  проб и общему bootstrap зафиксировано в `docs/adr/0017-shared-httpserver-and-readiness.md`.
