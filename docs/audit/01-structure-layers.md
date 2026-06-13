# Аудит SREOnCall — Область 1: Структура и слои

Дата: 2026-06-13
Область: `go.work`, project-layout, границы пакетов `internal/`, повторы кода между сервисами.
Применённые скилы: `golang-project-layout`, `golang-structs-interfaces`, `golang-design-patterns`.

Объём: 5 сервисов (escalation, incident, ingestion, notification, scheduling) + `pkg` + `tests/e2e`, ~7.6k строк не-тестового Go-кода в монорепо на `go.work` (Go 1.26.4).

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| F1 | **major** | Контракт шины событий продублирован 5+ раз — нет общего пакета. Структуры payload расходятся вручную | [escalation/publisher.go:13](services/escalation/internal/publisher/publisher.go#L13), [notifier.go:19](services/notification/internal/notifier/notifier.go#L19), [incident/publisher.go:11](services/incident/internal/publisher/publisher.go#L11), [escalation/consumer.go:14](services/escalation/internal/consumer/consumer.go#L14) |
| F2 | **major** | Слой персистентности/инфраструктуры живёт в `cmd/` у ingestion (SQL, Redis-адаптеры) — нарушение границ слоёв | [ingestion/cmd/server/main.go:125-165](services/ingestion/cmd/server/main.go#L125-L165) |
| F3 | **major** | Boilerplate HTTP-клиентов S2S скопирован 4 раза и уже разошёлся (один клиент не нормализует `baseURL`) | [escalation/schedclient:30](services/escalation/internal/schedclient/client.go#L30) vs [notification/schedclient:31](services/notification/internal/schedclient/client.go#L31) |
| F4 | **major** | Бутстрап `main()` (pool, migrate, topology, auth-toggle, shutdown) дублируется ×5 с расхождениями в таймаутах сервера | все `*/cmd/server/main.go` |
| F5 | **major** | scheduling строит логгер вручную и игнорирует `LOG_LEVEL` (поля нет в конфиге) — расхождение с остальными | [scheduling/main.go:25](services/scheduling/cmd/server/main.go#L25), [scheduling/config.go:6](services/scheduling/internal/config/config.go#L6) |
| F6 | **minor** | Хелперы чтения env (`getenv`/`envOr`/`getenvInt`/`envDurSec`) скопированы в каждый `config` с разными именами | [incident/config.go:25](services/incident/internal/config/config.go#L25) и др. |
| F7 | **minor** | Цикл консьюмера (qos/consume/select) дублируется ×3; внутри escalation `handle` и `ProcessDelivery` — копипаста switch | [escalation/consumer.go:63-122](services/escalation/internal/consumer/consumer.go#L63-L122) |
| F8 | **minor** | `incident/internal/domain` переопределяет `AlertStatus`, дублируя `pkg/domain.AlertStatus` | [incident/domain/incident.go:13](services/incident/internal/domain/incident.go#L13) vs [pkg/domain/alert.go:22](pkg/domain/alert.go#L22) |
| F9 | **minor** | escalation конструирует `escalator.New` дважды | [escalation/main.go:78](services/escalation/cmd/server/main.go#L78), [:89](services/escalation/cmd/server/main.go#L89) |
| F10 | **minor** | Блок «auth middleware или passthrough» скопирован дословно в 4 `main()` | [incident/main.go:71-84](services/incident/cmd/server/main.go#L71-L84) и др. |

Чисто структурных **critical** не выявлено — слабые места в безопасности/конкурентности будут разбираться в следующих областях. Главный системный риск здесь — **F1**: бизнес-контракт между сервисами не зафиксирован в одном месте, а размазан копипастой, что напрямую грозит «тихим» дрейфом схемы событий.

---

## Детализация

### F1 — Контракт шины событий не имеет единого источника правды — **major**

Один и тот же payload событий объявлен независимо в каждом сервисе:

- `escalation.triggered` / `escalation.exhausted` — определены как `TriggeredEvent`/`ExhaustedEvent` в [escalation/internal/publisher/publisher.go:13-29](services/escalation/internal/publisher/publisher.go#L13-L29) (продюсер) и **повторно** как `TriggeredEvent`/`ExhaustedEvent` в [notification/internal/notifier/notifier.go:19-35](services/notification/internal/notifier/notifier.go#L19-L35) (консьюмер). Это два независимых определения 9-полевой структуры, которые обязаны совпадать байт-в-байт по JSON-тегам.
- `incident.created` / `incident.updated` — `IncidentEvent` в [incident/internal/publisher/publisher.go:11-18](services/incident/internal/publisher/publisher.go#L11-L18) и **повторно** как приватный `incidentPayload` в [escalation/internal/consumer/consumer.go:14-21](services/escalation/internal/consumer/consumer.go#L14-L21).

Комментарии в коде сами признают риск дрейфа («mirrors the escalation.triggered payload», «Events from older … versions may carry an empty tenant_slug»). Сейчас синхронность держится только на дисциплине: добавление поля у продюсера никак не подсвечивается у консьюмера — компилятор молчит, потому что это разные типы в разных модулях.

`pkg/amqp` уже владеет транспортом (`Envelope`, `Wrap`/`Unwrap`, routing keys в [topology.go](pkg/amqp/topology.go)), но **payload-контракты** туда не вынесены.

**Фикс.** Завести `pkg/events` (или `pkg/contracts`) с каноническими типами payload и константами версий, рядом с routing-ключами:

```go
// pkg/events/events.go
package events

type EscalationTriggered struct { /* единственное определение 9 полей */ }
type EscalationExhausted struct { ... }
type IncidentChanged    struct { ... } // incident.created / incident.updated
```

Продюсеры и консьюмеры импортируют один тип. Локальные `notifier.TriggeredEvent`, `consumer.incidentPayload` и т.п. удаляются. Это согласуется со скилом `golang-design-patterns` (data handling / streaming): контракт между сервисами — общий тип, а не параллельные копии. Если намеренно держится слабая связанность по версиям — это решение стоит зафиксировать ADR (`docs/adr`), а не воспроизводить копипастой.

---

### F2 — Слой хранилища в `cmd/server/main.go` у ingestion — **major**

В отличие от остальных сервисов (везде есть `internal/store`), ingestion держит реализацию персистентности и инфраструктурных адаптеров прямо в `main`:

- `pgStore` + SQL `INSERT INTO ingestion.raw_alerts …` — [ingestion/cmd/server/main.go:159-172](services/ingestion/cmd/server/main.go#L159-L172);
- `redisCacheAdapter` (SetNX/Del) — [ingestion/cmd/server/main.go:131-141](services/ingestion/cmd/server/main.go#L131-L141);
- `redisTokenStore` (HGet) — [ingestion/cmd/server/main.go:145-152](services/ingestion/cmd/server/main.go#L145-L152).

По скилу `golang-project-layout` пакет `main` в `cmd/` должен только парсить конфиг, собирать зависимости и вызывать запуск; бизнес- и инфраструктурный код — в `internal/`. Сейчас SQL-схема `raw_alerts` непокрываема тестами пакета (всё в `package main`) и невидима из `internal/handler`, который от неё зависит через интерфейс.

**Фикс.** Вынести `pgStore` → `internal/store`, Redis-адаптеры → `internal/dedup` (или `internal/tokenstore`), оставив в `main` только конструкторы. Это выравнивает ingestion с остальными четырьмя сервисами.

---

### F3 — Дублирование и дрейф HTTP-клиентов S2S — **major**

Четыре почти одинаковых клиента «GET к соседнему сервису с `X-Admin-Key`»: [escalation/schedclient](services/escalation/internal/schedclient/client.go), [notification/schedclient](services/notification/internal/schedclient/client.go), [escalation/incclient](services/escalation/internal/incclient/client.go), [scheduling/keycloak](services/scheduling/internal/keycloak/client.go). Все повторяют один и тот же шаблон: `struct{baseURL, adminKey, httpClient}`, `http.Client{Timeout: 10s}`, установка заголовка, проверка статуса, `json.NewDecoder`.

Дрейф **уже произошёл**: escalation-клиент нормализует базовый URL — `strings.TrimRight(baseURL, "/")` ([escalation/schedclient/client.go:30](services/escalation/internal/schedclient/client.go#L30)), а notification-клиент пишет `baseURL: baseURL` без нормализации ([notification/schedclient/client.go:31](services/notification/internal/schedclient/client.go#L31)). Итог: trailing slash в `SCHEDULING_URL` ломает notification, но не ломает escalation — ровно тот класс расхождений, который копипаста и порождает. Различается и обёртка ошибок (`schedclient: build request:` vs голый `err`).

**Фикс.** Базовый клиент в `pkg/httpclient`: нормализация URL, инъекция admin-key, единый разбор статуса/ошибок (`golang-structs-interfaces`: «accept interfaces, return structs», маленький переиспользуемый тип). Сервисные клиенты оборачивают его и описывают только эндпойнты и DTO.

---

### F4 — Дублирующийся бутстрап `main()` с расходящимися таймаутами — **major**

~100 строк инициализации повторяются в каждом `cmd/server/main.go`: `pkgdb.NewPool` → `pkgmigrate.Run` → `NewConnection`/`Channel`/`DeclareTopology`/`Close` → toggle auth-middleware → `http.Server` + горутина graceful shutdown.

Расхождения, которые это уже породило:

- Таймауты сервера несогласованы: incident и ingestion задают `ReadTimeout/WriteTimeout/IdleTimeout` ([incident/main.go:118-123](services/incident/cmd/server/main.go#L118-L123)), а escalation и notification — только `ReadHeaderTimeout` ([escalation/main.go:142](services/escalation/cmd/server/main.go#L142), [notification/main.go:142](services/notification/cmd/server/main.go#L142)). То есть у двух сервисов нет `WriteTimeout`/`IdleTimeout`.
- Порядок запуска сервера разный: incident/ingestion вызывают `ListenAndServe` в основной горутине и шатдаунят из фоновой; escalation/notification — наоборот.

**Фикс.** Хелпер `pkg/httpserver` (или `pkg/bootstrap`) с функцией `Run(ctx, addr, handler)` и едиными таймаутами + graceful shutdown; опционально `pkg/amqp.Setup(url)` для связки connection+topology. Скил `golang-design-patterns` (graceful shutdown / lifecycle) описывает ровно этот паттерн как единую точку.

---

### F5 — scheduling игнорирует уровень логирования и расходится с остальными — **major**

Все сервисы инициализируют логгер через `pkglogger.New(cfg.LogLevel)`. scheduling вместо этого собирает логгер вручную и без уровня:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)) // scheduling/main.go:25
```

При этом в `scheduling/internal/config/config.go` **поля `LogLevel` вообще нет** ([config.go:5-17](services/scheduling/internal/config/config.go#L5-L17)) — переменная `LOG_LEVEL` в этом сервисе не действует, хотя задокументирована для других. Это и нарушение единообразия слоя конфигурации, и функциональное расхождение (нельзя поднять/опустить уровень логов в проде).

**Фикс.** Добавить `LogLevel` в конфиг scheduling и перейти на `pkglogger.New(cfg.LogLevel)`, как в остальных четырёх сервисах.

---

### F6 — Хелперы env скопированы в каждый `config` под разными именами — **minor**

Одна и та же функция «env или дефолт» существует как `getenv` ([incident:25](services/incident/internal/config/config.go#L25), [escalation:38](services/escalation/internal/config/config.go#L38), [notification:33](services/notification/internal/config/config.go#L33), [scheduling:34](services/scheduling/internal/config/config.go#L34)) и как `envOr` ([ingestion:35](services/ingestion/internal/config/config.go#L35)). Дополнительно `getenvInt` ([notification:40](services/notification/internal/config/config.go#L40)) и `envDurSec` ([ingestion:42](services/ingestion/internal/config/config.go#L42)) — каждый в одном экземпляре, но того же семейства. Разнобой имён (`getenv` vs `envOr`) — лишний штрих к копипасте.

**Фикс.** `pkg/config` (или `pkg/env`) с `String`, `Int`, `DurationSeconds`. Сервисные `config.Load()` остаются, но используют общие примитивы. Заодно устраняется несогласованность парсинга int: notification использует `fmt.Sscanf` с трактовкой `0` как ошибки ([notification/config.go:40-48](services/notification/internal/config/config.go#L40-L48)), что заодно отвергает легитимный `0`.

---

### F7 — Дублирование цикла консьюмера и внутренняя копипаста — **minor**

Цикл `Channel → Qos(10,0,false) → Consume → for/select{ctx.Done, msg}` идентичен в [notification/consumer.go:23-50](services/notification/internal/consumer/consumer.go#L23-L50), [escalation/consumer.go:34-61](services/escalation/internal/consumer/consumer.go#L34-L61) и incident-консьюмере. Кроме того, внутри escalation-консьюмера `handle` ([:63](services/escalation/internal/consumer/consumer.go#L63)) и `ProcessDelivery` ([:102](services/escalation/internal/consumer/consumer.go#L102)) содержат две копии одного `switch` по типу события — расходятся в ack/nack-логике, но повторяют маршрутизацию.

Отдельно: notification-консьюмер сначала `json.Unmarshal` для конверта, затем `pkgamqp.Unwrap`, который **повторно** анмаршалит тот же конверт ([notification/consumer.go:67-77](services/notification/internal/consumer/consumer.go#L67-L77)) — двойной разбор.

**Фикс.** Вынести каркас «consume + ack/nack» в `pkg/amqp.Consume(ctx, conn, queue, handlerFunc)`, где сервис передаёт только функцию обработки тела. В escalation свести `handle` к вызову `ProcessDelivery` + общий ack/nack. `pkgamqp.Unwrap` уже возвращает `Envelope` — отдельный `json.Unmarshal` не нужен.

---

### F8 — `incident/domain` дублирует `pkg/domain.AlertStatus` — **minor**

[incident/internal/domain/incident.go:13-18](services/incident/internal/domain/incident.go#L13-L18) объявляет свой `AlertStatus` с `firing`/`resolved`, что повторяет канонический `pkg/domain.AlertStatus` (`AlertStatusFiring`/`AlertStatusResolved`, [pkg/domain/alert.go:22-27](pkg/domain/alert.go#L22-L27)). Два типа с одинаковыми строковыми значениями и смыслом — риск рассинхронизации значений между сервисом ingestion (издатель) и incident (потребитель).

**Фикс.** Переиспользовать `pkg/domain.AlertStatus` в incident, либо явно конвертировать на границе. Локальные доменные перечисления incident (`Status` open/ack/resolved) — оставить, они инцидент-специфичны.

---

### F9 — Двойная конструкция `escalator.New` в escalation/main — **minor**

[escalation/main.go:78](services/escalation/cmd/server/main.go#L78) создаёт `esc` внутри `if cfg.AMQPURL != ""` (только для консьюмера), затем [:89](services/escalation/cmd/server/main.go#L89) создаёт ещё один `esc` для HTTP-хендлера и монитора. Два экземпляра одного оркестратора со связанным состоянием — структурный запах от того, что проводка живёт в линейном `main`.

**Фикс.** Создать `esc` один раз до ветвления и передать его и в `consumer.New`, и в `handler.New`/`monitor.New`.

---

### F10 — Повтор toggle auth-middleware — **minor**

Блок «если `KeycloakJWKSURL` задан — `pkgauth.Middleware`, иначе passthrough» скопирован дословно в incident ([:71-84](services/incident/cmd/server/main.go#L71-L84)), escalation, notification и scheduling.

**Фикс.** Добавить в `pkg/auth` конструктор вида `MiddlewareOrPassthrough(jwksURL, adminKey) (func(http.Handler) http.Handler, error)`, возвращающий passthrough при пустом URL.

---

## Что со структурой сделано хорошо (для контекста)

- `go.work` + per-module `replace github.com/sre-oncall/pkg => ../../pkg` ([escalation/go.mod:30](services/escalation/go.mod#L30) и др.) — корректно: модули собираются и внутри воркспейса, и автономно.
- Раскладка `cmd/server` + `internal/{handler,store,domain,config,...}` единообразна и соответствует `golang-project-layout` (кроме F2 в ingestion).
- Имена модулей (`github.com/sre-oncall/<service>`) — lowercase, семантичные.
- Транспортный слой шины (`pkg/amqp`: `Envelope`/`Wrap`/`Unwrap`, топология) уже централизован — это правильная база, на которую логично «дотащить» и контракты payload (F1).

---

## Рекомендованный порядок исправлений

1. **F1** — `pkg/events`: убирает системный риск дрейфа контракта (наибольший эффект).
2. **F5** — быстрый фикс реального функционального расхождения (LOG_LEVEL в scheduling).
3. **F3** — `pkg/httpclient`: закрывает уже проявившийся баг с `baseURL`.
4. **F4 + F6 + F10** — `pkg/httpserver` / `pkg/config` / `pkgauth` helper: убирают основной объём копипасты в `main`.
5. **F2, F7, F8, F9** — точечные рефакторинги границ слоёв и дедуп внутри сервисов.
