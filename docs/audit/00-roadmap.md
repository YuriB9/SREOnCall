# SREOnCall — Roadmap исправлений по итогам аудита

Дата: 2026-06-13
Назначение: план реализации находок аудита (`docs/audit/01..11`) через OpenSpec-чейнджи.
Стратегия: **много маленьких чейнджей, сгруппированных по корню проблемы** (не «один чейндж = одна находка», не «один мега-чейндж»). Соответствует норме проекта (`harden-auth-shell`, `split-notification-config-mattermost-email`).

## Принципы нарезки и порядка

1. **Энейблеры вперёд** — CI и общие `pkg/*` дают страховочную сетку и уменьшают диффы последующих чейнджей.
2. **Безопасность/корректность раньше рефакторингов «под стиль»**.
3. **Рискованное поведение (горутины, транзакции) — отдельным изолированным чейнджем** ради ревью и отката.
4. **Уважать зависимости** (например, `/readyz` с проверкой живости консьюмера — после переделки консьюмера).
5. **Стиль/доки/мелочь — в основном энфорсятся линтером из CH01**, отдельные чейнджи только там, где нужна ручная правка.

Легенда риска: 🟢 низкий (аддитивно/механически) · 🟡 средний (трогает много мест) · 🔴 высокий (поведенческое изменение).

---

## Статус (дашборд)

Единая точка отслеживания. Сессия, завершившая чейндж, ставит `✅` здесь **и** в строке чейнджа ниже.
Статусы: `☐ todo` · `🔄 in progress` · `✅ done` · `⏸ blocked` (ждёт зависимость).

| CH | Чейндж | Фаза | Зависит от | Статус |
| --- | --- | --- | --- | --- |
| CH01 | add-ci-and-quality-gates | 0 | — | ✅ |
| CH02 | bump-vulnerable-auth-deps | 0 | — | ✅ |
| CH03 | harden-auth-validation | 1 | CH01 | ✅ |
| CH04 | guard-webhook-ssrf | 1 | CH01 | ✅ |
| CH05 | extract-bus-contracts | 2 | CH01 | ✅ |
| CH06 | extract-shared-config-and-httpclient | 2 | CH01 | ✅ |
| CH07 | consumer-resilience | 3 | CH05 | ✅ |
| CH08 | db-atomicity-and-state-transitions | 3 | CH01 | ☐ |
| CH09 | store-layering-and-pool-config | 3 | CH01 | ☐ |
| CH10 | shared-httpserver-and-readiness | 4 | CH07, CH03 | ☐ |
| CH11 | pipeline-metrics-and-alerts | 5 | CH10, CH07 | ☐ |
| CH12 | log-correlation | 5 | CH10 | ☐ |
| CH13 | distributed-tracing | 5 | CH05, CH10 | ☐ |
| CH14 | bus-publish-perf | 6 | CH07 | ☐ |
| CH15 | ingestion-throughput | 6 | CH09, CH11 | ☐ |
| CH16 | tenantcache-singleflight | 6 | CH01 | ☐ |
| CH17 | test-hardening | 7 | CH01 | ☐ |
| CH18 | docs-and-style | 7 | CH01 | ☐ |
| CH19 | containerize-and-scan | 7 | CH01 | ☐ |

Прогресс: **7 / 19** done.

---

## Фаза 0 — Фундамент и срочная безопасность

### CH01 · `add-ci-and-quality-gates` 🟢 — ✅ done (2026-06-13)
**Корень:** нет автоматического гейта качества.
**Закрывает:** T1, DC2, DC3, DC4, T2 (снять тег `integration` со стаб-тестов → дефолтный прогон снова что-то охраняет), DC6 (e2e `go.sum`).
**Содержимое:** GitHub Actions (матрица по модулям `go.work`): `go test -race -shuffle=on`, `go mod tidy && git diff --exit-code`, `golangci-lint`, `govulncheck`, отдельный integration-джоб с сервисами; `.golangci.yml` (errcheck, govet, staticcheck, revive, gosec, bodyclose, sqlclosecheck, nilerr, modernize, errname, paralleltest); `tool`-директивы в go.mod; Renovate/Dependabot.
**Зависит от:** —. **Первый** — даёт сетку для всего остального.

> **Реализовано.** Чейндж `add-ci-and-quality-gates` (архив с `--skip-specs`, no-delta tooling).
> Что важно для следующих сессий:
> - **golangci-lint в CI = `only-new-issues`** (на PR `--new-from-merge-base`, на push в main — информационно). Baseline 188 замечаний — это долг профильных чейнджей: paralleltest→CH17, revive/modernize/package-comments→CH18, errcheck `_ =`→CH08/CH18. Они **не блокируют**, пока не правишь их файлы.
> - **Инструменты пинятся изолированным модулем `./tools`** (golangci-lint v2.12.2, govulncheck v1.3.0; вне `go.work`). Менять версии — там; CI собирает бинари из него.
> - **Стаб-тесты переименованы** `*_integration_test.go` → `*_test.go` и теперь в дефолтном прогоне. Тег `integration` оставлен только у `scheduling/store` и `scheduling/tokenindex`. Чинились 2 протухших теста notification (см. tasks.md §2.4).
> - **e2e.yml — только `workflow_dispatch`** (полный стек не доведён до зелёного; `schedule` отключён). Довести вероятно вместе с CH19.
> - **Backlog для CH17/T5:** `go vet` httpresponse в `handler_test.go` (escalation/incident/scheduling) — латентный nil-deref, вскрыт снятием тега, отложен по объёму.
> - **Git-воркфлоу с этого момента — через PR** (Actions настроен): мерж по зелёному CI.

### CH02 · `bump-vulnerable-auth-deps` 🟢 — ✅ done (2026-06-13)
**Корень:** достижимые CVE в дереве зависимостей.
**Закрывает:** DC1 (jwt/v5 → v5.2.2 [GO-2025-3553], jwkset → v0.11.0 [GO-2025-3376]).
**Содержимое:** бамп версий в каждом модуле с auth, `go mod tidy`, перепроверка `govulncheck`.
**Зависит от:** —. Срочно (две достижимые уязвимости в пути auth); верифицируется гейтом из CH01.

> **Реализовано.** Чейндж `bump-vulnerable-auth-deps` (no-delta infra/tooling, архив с `--skip-specs`).
> Что важно для следующих сессий:
> - **`jwkset v0.6.0` из аудита ретрактнута автором** (гонка в refresh, PR #42), а текущая `keyfunc/v3 v3.3.5` сама в ретракт-диапазоне `[v3.0.0, v3.3.5]` по GO-2025-3376. Фикс взят через бамп **`keyfunc/v3 v3.3.5 → v3.8.0`**, который тянет неретракт-фикс **`jwkset v0.5.19 → v0.11.0`** и **`jwt/v5 v5.2.1 → v5.2.2`**.
> - `govulncheck ./...` — **0 vulnerabilities** во всех 6 модулях. `services/ingestion` auth не использует (auth-зависимостей нет).
> - **Джоб `govulncheck` в CI теперь блокирующий** (снят `continue-on-error`). Любой новый достижимый адвизори валит CI.
> - Go-исходники не менялись — только `go.mod`/`go.sum` + `ci.yml`.

---

## Фаза 1 — Безопасность (маленькие, изолированные)

### CH03 · `harden-auth-validation` 🟡 — ✅ done (2026-06-13)
**Корень:** ослабленная аутентификация.
**Закрывает:** S1 (fail-closed при пустом JWKS + явный флаг `AUTH_DISABLED` для локалки), S3 (`subtle.ConstantTimeCompare` для admin-key), S4 (`jwt.WithAudience/WithIssuer/WithValidMethods/WithExpirationRequired`), S5 (форс https для JWKS).
**Содержимое:** правки `pkg/auth` + конфиг (issuer/audience). **Риск:** fail-closed может «уронить» мисконфигнутый деплой — это и есть цель, но нужен escape-hatch и явная миграция env.
**Зависит от:** CH01 (гейт), желательно после CH02.

> **Реализовано.** Чейндж `harden-auth-validation` (no-delta infra/security, архив с `--skip-specs`). См. ADR-0012.
> Что важно для следующих сессий:
> - **`pkg/auth.Middleware` сменил сигнатуру** на `Middleware(auth.Options{JWKSURL, AdminKey, Issuer, Audience, AllowInsecureJWKS})` — учесть в **CH10** (F10/F4: wiring auth выносится в `pkg/httpserver`). Fail-closed-блок сейчас продублирован в 4 `main.go` единообразно — готов к выносу.
> - **Fail-closed:** пустой `KEYCLOAK_JWKS_URL` → `os.Exit(1)`; отключение только `AUTH_DISABLED=true`. Локальный запуск Go-сервисов (вне docker-compose) и CI без Keycloak требуют `AUTH_DISABLED=true`.
> - **S5 force-https:** k8s-configmap'ы тянут JWKS по http (`http://keycloak:8080`), поэтому проставлен `AUTH_INSECURE=true` с пометкой перевести на https.
> - **iss/aud — enforce-if-configured:** включаются только при заданных `KEYCLOAK_ISSUER`/`KEYCLOAK_AUDIENCE` (в манифестах закомментированы). **Backlog:** сделать iss/aud обязательными после раскатки env; сузить полномочия god-key admin-key / ротация / client-credentials (S3-follow-up, ADR-0009).
> - `ingestion` вне объёма (webhook-token auth, JWT-middleware не использует).
> - Проверки: `go build/vet/test` всех модулей, `-race ./pkg/auth`, `golangci-lint` (0 new issues), `govulncheck` (0 достижимых) — чисто.

### CH04 · `guard-webhook-ssrf` 🟢 — ✅ done (2026-06-13)
**Корень:** SSRF через подконтрольный тенанту URL.
**Закрывает:** S2 (валидация Mattermost webhook URL: только https + блок приватных/loopback/link-local — на записи в scheduling и на отправке в notification).
**Зависит от:** CH01. Самостоятельный.

> **Реализовано.** Чейндж `guard-webhook-ssrf`. См. ADR-0013.
> Что важно для следующих сессий:
> - **Новый общий пакет `pkg/ssrf`**: `ValidateURL(ctx, raw)` (https + резолв + блок
>   private/loopback/link-local/unspecified/multicast/ULA) и `GuardedDialContext(base)`
>   (блок приватного IP на дозвоне — ловит TOCTOU/DNS-rebinding/редиректы). Sentinel-ошибки
>   `ErrNotHTTPS`/`ErrBlockedAddress`/`ErrInvalidURL`. Переиспользуем для других исходящих
>   тенант-задаваемых URL.
> - **scheduling** валидирует непустой `mattermost_webhook_url` на PUT → **422** на небезопасный URL
>   (`handler.go` `PutTenantNotificationConfig`). **Только https** — http-вебхуки больше не принимаются.
> - **notification** применяет `GuardedDialContext` к http-клиенту Mattermost-диспетчера
>   (`dispatcher/mattermost.go`) — defense in depth для уже сохранённых URL.
> - **Стратегия — блок приватных диапазонов** (не allowlist доменов): работает с self-hosted
>   Mattermost. Уже сохранённые небезопасные URL не мигрируются — блокируются на отправке (`failed`).
> - **Тесты офлайн:** в хендлер-тестах webhook-URL заменены на литеральный публичный IP `203.0.113.10`
>   (TEST-NET-3), чтобы валидатор не дёргал DNS. Учесть в новых тестах, трогающих эти пути.
> - Проверки: `go build/vet/test` (pkg/scheduling/notification), `golangci-lint` (0 new issues),
>   `govulncheck` (0 достижимых) — чисто.

---

## Фаза 2 — Общие `pkg/*` энейблеры (сокращают диффы дальше)

### CH05 · `extract-bus-contracts` 🟡 — ✅ done (2026-06-13)
**Корень:** контракт шины размазан копипастой.
**Закрывает:** F1 (`pkg/events` — канонические payload'ы событий), F8 (incident переиспользует `pkg/domain.AlertStatus`), часть N4 (согласование имён статусов между пакетами).
**Содержимое:** новый `pkg/events`, удаление дублей `TriggeredEvent`/`ExhaustedEvent`/`IncidentEvent`/`incidentPayload`. Механически, но трогает publisher/consumer всех сервисов.
**Зависит от:** CH01. Делать **до** CH07 (консьюмеры начнут использовать канонические типы).

> **Реализовано.** Чейндж `extract-bus-contracts` (no-delta infra-рефактор, архив с `--skip-specs`). См. ADR-0014.
> Что важно для следующих сессий:
> - **Новый пакет `pkg/events`** (модуль `github.com/sre-oncall/pkg`, без нового go.mod) — единственный источник правды payload'ов: `EscalationTriggered` (`escalation.triggered`), `EscalationExhausted` (`escalation.exhausted`), `IncidentChanged` (`incident.created`/`updated`). Продюсеры/консьюмеры/интерфейсы `Publisher` уже ссылаются на эти типы; локальные дубли удалены.
> - **Для CH07** (`consumer-resilience`): консьюмеры всех сервисов `Unwrap` в `events.*` — переработку `pkg/amqp.Consume` строить вокруг этих канонических типов, не возвращать локальные payload-структуры.
> - **Для CH13** (`distributed-tracing`): проброс trace-context — через `pkg/amqp.Envelope`, payload'ы (`pkg/events`) не трогать.
> - **Wire-формат НЕ менялся** (JSON-теги перенесены 1:1) — **не BREAKING**, сообщения в очередях/от старых версий читаются как прежде. Смена JSON-тегов в `pkg/events` в будущем = BREAKING для очередей (помечать).
> - **F8/N4:** `incident/internal/domain` больше не объявляет `AlertStatus` — используется `pkg/domain.AlertStatus` (`AlertStatusFiring`/`AlertStatusResolved`). Инцидент-специфичный `domain.Status` (open/ack/resolved) остался. `alert.received` вне области (уже несёт `pkg/domain.Alert`).
> - **Остаток N4** (zero-value `Unknown`, префиксы sentinel'ов) — НЕ трогался, остаётся за **CH18**.
> - Проверки: `go build/vet/test` (pkg/escalation/incident/notification), `golangci-lint --new-from-merge-base main` (0 new issues), `go mod tidy` без диффа — чисто. Предсуществующие `go vet` httpresponse-замечания в `*/handler_test.go` — backlog T5 (CH17), не трогались.

### CH06 · `extract-shared-config-and-httpclient` 🟢 — ✅ done (2026-06-13)
**Корень:** дублирование инфраструктурного кода + расхождение клиентов.
**Закрывает:** F6 (`pkg/config` env-хелперы), F3 (`pkg/httpclient` базовый клиент), E3 (общие sentinel'ы + маппинг 404→`ErrNotFound`/409→`ErrConflict` в клиентах), P4 (настроенный общий `http.Transport`).
**Содержимое:** `pkg/config`, `pkg/httpclient`, `pkg/errs` (общие sentinel'ы); перевод schedclient/incclient/keycloak на базовый клиент.
**Зависит от:** CH01.

> **Реализовано.** Чейндж `extract-shared-config-and-httpclient` (no-delta infra-рефактор, архив с `--skip-specs`).
> Что важно для следующих сессий:
> - **`pkg/config`** (F6): `String`/`Int`/`DurationSeconds`. `Int`/`DurationSeconds` через `strconv.Atoi` — принимают легитимный `0` (фикс notification, где `fmt.Sscanf` отвергал `0`). Все 5 `config.Load()` импортируют как `pkgconfig` (имя сервис-пакета тоже `config`).
> - **`pkg/errs`** (E3): канонические `ErrNotFound`/`ErrConflict` на монорепо. Store-сентинелы incident/escalation/scheduling/notification теперь **алиасы** (`var ErrNotFound = errs.ErrNotFound`) — `errors.Is(err, store.ErrNotFound)` и `OverrideConflictError.Is` работают как прежде, но значение одно. **Для CH08/CH10:** возвращай `errs.*` (или store-алиас) — не вводи новые локальные sentinel'ы.
> - **`pkg/httpclient`** (F3+E3+P4): `New(baseURL, adminKey)` (нормализует `baseURL` — фикс дрейфа notification на trailing slash) + `GetJSON(ctx, path, out)` (маппит `404→errs.ErrNotFound`, `409→errs.ErrConflict`, прочее → `%w`). schedclient/incclient (escalation) и schedclient (notification) — тонкие обёртки. `NewStdClient(timeout)` отдаёт `*http.Client` на **общем тюнингованном Transport** (`MaxIdleConnsPerHost=50`) — keycloak на нём; mattermost получил те же idle-настройки, **сохранив** SSRF-guarded dialer из CH04. **Для CH10/CH13/CH14:** новые S2S-клиенты строй на `pkg/httpclient`.
> - **notification/schedclient** сохранил контракт «404 → (nil, nil)» через `errors.Is(err, errs.ErrNotFound)`.
> - **Wire-формат, API, события RabbitMQ НЕ менялись** — не BREAKING.
> - Проверки: `go build/vet/test` всех 6 модулей, `golangci-lint --new-from-merge-base main` (0 new), `govulncheck` (0 достижимых), `go mod tidy` без диффа — чисто. Предсуществующие `go vet` httpresponse-замечания в `*/handler_test.go` — backlog T5 (CH17), не трогались.

---

## Фаза 3 — Операционная корректность (высшая серьёзность, 🔴 изолированно)

### CH07 · `consumer-resilience` 🔴 — ✅ done (2026-06-13)
**Корень:** жизненный цикл фоновых горутин.
**Закрывает:** C1 (supervisor-петля переподключения консьюмера), C2 (`errgroup` graceful-drain + `defer amqpConn.Close()`), E2 (`recover` в обработке сообщения), C3 (drain-контекст `WithoutCancel`+timeout для in-flight), C4 (не держать мьютекс через `time.Sleep` в `Channel`), C5 (отменяемый backoff), C6 (`Publish` использует переданный `ctx`), C8 (bounded worker-pool вместо холостого `Qos(10)`), F7 (`pkg/amqp.Consume` — общий каркас consume/ack/nack).
**Содержимое:** переработка `pkg/amqp` + консьюмеров всех сервисов + разводки в `main`.
**Зависит от:** CH05 (канонические типы). **Самый важный операционно** (тихая смерть конвейера). Изолировать.

> **Реализовано.** Чейндж `consumer-resilience` (no-delta infra/reliability, архив без spec-дельты). См. ADR-0015.
> Что важно для следующих сессий:
> - **Новый каркас `pkg/amqp.Consume(ctx, conn, opts, handler)`** (`pkg/amqp/consume.go`) — единый цикл consume/ack/nack: supervisor-реконнект (C1), `recover` на сообщение → drop (E2), drain-контекст `WithTimeout(WithoutCancel(ctx), 30s)` на сообщение (C3), bounded worker-pool `errgroup.SetLimit` с `prefetch=Concurrency` (C8). `Handler func(ctx, Envelope) error`: `nil`→Ack, ошибка→Nack+requeue, `amqp.Drop(err)`→Nack без requeue. Helper `DecodePayload(env, dst)`. **Все новые консьюмеры строить на нём**, не копировать цикл.
> - **`Connection.Channel()` → `Channel(ctx)`** (смена сигнатуры pkg, обновлены все 5 `main.go` + ingestion). Реконнект не держит `mu` через сон (`reconnectMu`+double-check, C4). `Publish` пробрасывает `ctx` в `PublishWithContext` (C6). **Для CH14** (`bus-publish-perf`): переиспользуемый канал строить на этом `Connection`.
> - **Graceful-drain в main** (incident/escalation/notification): фоновые горутины в `errgroup`, `g.Wait()` после `srv.Shutdown`, затем `amqpConn.Close()` (C2). **Для CH10** (`/readyz` живость консьюмера): сигнал состояния консьюмера завести вокруг `pkg/amqp.Consume` (точка для метрики/healthcheck — здесь не делалось, `/readyz` остался статическим).
> - **Семантика подтверждений сохранена:** incident/escalation requeue'ят сбои обработки, drop'ают невалидный конверт/панику; notification drop'ает любую ошибку (через `Drop()`). **DLQ — вне объёма** (отдельная находка надёжности шины).
> - **Дефолт `Concurrency=1`** (строго последовательно) — сохраняет порядок `incident.created`→`incident.updated`, критичный для escalation. Параллелизм — осознанный opt-in через `ConsumeOptions.Concurrency`.
> - Диспетчеры notification (Mattermost/email) и supervisor-backoff: `time.Sleep` → отменяемое ожидание по `ctx` (C5); email-диспетчер теперь использует `ctx`.
> - **Wire-формат `Envelope`/payload НЕ менялся** — не BREAKING, сообщения в очередях читаются как прежде.
> - Проверки: `go build/vet/test` всех 6 модулей, **`-race`** (чисто), `golangci-lint --new-from-merge-base main` (0 new), `govulncheck` (0 достижимых), `go mod tidy` (`golang.org/x/sync`→direct). Предсуществующие `go vet` httpresponse-замечания в `*/handler_test.go` — backlog T5 (CH17), не трогались.

### CH08 · `db-atomicity-and-state-transitions` 🔴
**Корень:** нет оптимистичной конкуренции на переходах состояний.
**Закрывает:** D1 (`FOR UPDATE SKIP LOCKED` в транзакции **или** CAS-`UPDATE ... WHERE status/tier=expected` + проверка `RowsAffected`), D3 (тот же CAS для `PatchStatus`), D2 (транзакционный `withTx`-хелпер для составных записей incident/escalation), D5 (keyset-пагинация `(created_at, id)`), E4 (логировать ошибку `AppendHistory` вместо `_ =`), R2 (проверять err/nil в post-write чтениях хендлера).
**Содержимое:** store-слой incident/escalation + хендлеры.
**Зависит от:** CH01 (а на проде — лучше после CH07, чтобы requeue работал чисто). Изолировать.

### CH09 · `store-layering-and-pool-config` 🟡
**Корень:** слой персистентности в `cmd/` + ненастроенный пул.
**Закрывает:** F2 (вынести `pgStore`/redis-адаптеры/SQL из `ingestion/cmd/server/main.go` в `internal/`), D4 (конфиг пула `MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime` в `pkg/db`).
**Зависит от:** CH01.

---

## Фаза 4 — Консолидация HTTP-сервера

### CH10 · `shared-httpserver-and-readiness` 🟡
**Корень:** разнобой bootstrap/middleware между сервисами.
**Закрывает:** F4 (`pkg/httpserver` — сервер с едиными таймаутами + graceful shutdown), E1 (`Recoverer` во всех 5 сервисах), F10 (auth-toggle в общий хелпер), O6 (`RequestID` везде), S6 (входной rate-limit middleware), F5 (scheduling → `pkglogger.New(cfg.LogLevel)` + поле `LogLevel`), F9 (один `escalator.New`), O1 (content-aware `/readyz`: БД/Redis/AMQP + живость консьюмера), E6 (не отдавать `err.Error()` клиенту).
**Содержимое:** новый `pkg/httpserver`, переразводка всех `main.go`.
**Зависит от:** CH07 (сигнал живости консьюмера для `/readyz`), CH03 (auth-wiring собирается тут же).

---

## Фаза 5 — Наблюдаемость

### CH11 · `pipeline-metrics-and-alerts` 🟡
**Корень:** конвейер слеп для метрик.
**Закрывает:** O2 (доменные + шинные метрики: alerts/incidents/escalations/notifications, ack/nack/requeue, длительность обработки, backlog, publish-ошибки, `pgxpool.Stat`), R1 (метить запрос `chi RoutePattern`, а не `r.URL.Path` — фикс кардинальности), O5 (`ServiceMonitor` + `PrometheusRule` золотых сигналов + базовые дашборды).
**Зависит от:** CH10 (метрик-middleware живёт там), CH07 (есть что мерить по консьюмеру).

### CH12 · `log-correlation` 🟢
**Корень:** логи без корреляции.
**Закрывает:** O4 (`slog.*Context` + инъекция `request_id`/`trace_id` в записи), E5 (единый ключ `"err"`).
**Зависит от:** CH10 (request_id middleware). Часть `trace_id` подключится после CH13.

### CH13 · `distributed-tracing` 🟡
**Корень:** нет сквозной трассировки.
**Закрывает:** O3 (OpenTelemetry: `otelhttp`, span'ы на store/клиенты, **проброс trace-context через `pkg/amqp.Envelope`**).
**Зависит от:** CH05 (конверт), CH10. Крупный — отдельным чейнджем, можно позже.

---

## Фаза 6 — Производительность (после метрик/бенчей)

### CH14 · `bus-publish-perf` 🟡
**Корень:** канал AMQP пересоздаётся на каждую публикацию.
**Закрывает:** P1 (переиспользуемый канал/пул на `Publisher`).
**Зависит от:** CH07 (там уже переработан `pkg/amqp`), CH01 (бенчмарки). Требует benchstat до/после в коммите.

### CH15 · `ingestion-throughput` 🟡
**Корень:** последовательный I/O без батчинга.
**Закрывает:** P2 (батч-INSERT `raw_alerts` + пайплайн Redis + воркер-пул), P3 (multi-row INSERT в `MergeLabels`), P5 (один `json.Marshal` для `Alert`).
**Зависит от:** CH09 (store вынесен), CH11 (метрики для замера), CH01 (бенчи).

### CH16 · `tenantcache-singleflight` 🟢
**Корень:** stampede + неограниченный рост кэша.
**Закрывает:** C7 (`singleflight` вокруг fetch + вытеснение протухших ключей).
**Зависит от:** CH01. Маленький, самостоятельный.

---

## Фаза 7 — Тест/стиль-гигиена (низкий приоритет, многое уже энфорсит линтер)

### CH17 · `test-hardening` 🟢
**Закрывает:** T3 (`goleak` в `consumer`/`monitor`/`pkg/amqp`), T4 (юниты на `pkg/amqp.Envelope`, `tenantcache`, store эскалации — регресс-гард к D1), T5 (`t.Parallel()` + table-driven для парсера/матрицы).
**Зависит от:** CH01 (CI их гоняет), CH07/CH08 (тесты на новые гарантии).

### CH18 · `docs-and-style` 🟢
**Закрывает:** N1 (package-комментарии во все пакеты), N3 (`Deps`/options для `notifier.New`; разнести крупные хендлеры по файлам), N4 (`SeverityUnknown` + логировать неизвестную severity вместо тихого `Info`; префикс пакета в sentinel-строках), N5 (`[]any`, LICENSE/CONTRIBUTING), N2 (стуттер — точечно/по соглашению).
**Зависит от:** CH01 (линтер подсветит бо́льшую часть).

### CH19 · `containerize-and-scan` 🟢
**Закрывает:** DC5 (multi-stage `Dockerfile` на сервис, distroless/non-root, пин base; `docker.yml` с Trivy + SBOM).
**Зависит от:** CH01.

---

## Критический путь и параллелизм

```
CH01 ─┬─ CH02 ───────────────────────────────────► (срочно)
      ├─ CH03 ─┐
      ├─ CH04  │
      ├─ CH05 ─┼─ CH07 ─┬─ CH10 ─┬─ CH11 ─ CH12
      ├─ CH06 ─┘        │        └─ CH13
      │        CH08 ────┤        
      │        CH09 ────┴─ CH15
      │        CH14 (после CH07)
      ├─ CH16
      ├─ CH17 (после CH07/CH08)
      ├─ CH18
      └─ CH19
```

- **После CH01** независимы и параллелятся: CH02, CH03, CH04, CH05, CH06, CH09, CH16, CH18, CH19.
- **Критический путь** (самый длинный): CH01 → CH05 → CH07 → CH10 → CH11 → CH12.
- **Самые ценные операционно:** CH02 (срочно), CH07 и CH08 (тяжёлые баги: тихая смерть консьюмера, двойная эскалация).

## Матрица покрытия находок

| Чейндж | Находки |
|--------|---------|
| CH01 | T1, T2, DC2, DC3, DC4, DC6 |
| CH02 | DC1 |
| CH03 | S1, S3, S4, S5 |
| CH04 | S2 |
| CH05 | F1, F8, N4(имена статусов) |
| CH06 | F3, F6, E3, P4 |
| CH07 | C1, C2, C3, C4, C5, C6, C8, E2, F7 |
| CH08 | D1, D2, D3, D5, E4, R2 |
| CH09 | F2, D4 |
| CH10 | F4, F5, F9, F10, E1, E6, O1, O6, S6 |
| CH11 | O2, O5, R1 |
| CH12 | O4, E5 |
| CH13 | O3 |
| CH14 | P1 |
| CH15 | P2, P3, P5 |
| CH16 | C7 |
| CH17 | T3, T4, T5 |
| CH18 | N1, N2, N3, N4, N5 |
| CH19 | DC5 |

Все находки 11 областей покрыты. 19 чейнджей; ~9 параллелятся сразу после CH01.
