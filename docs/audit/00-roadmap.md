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
| CH08 | db-atomicity-and-state-transitions | 3 | CH01 | ✅ |
| CH09 | store-layering-and-pool-config | 3 | CH01 | ✅ |
| CH10 | shared-httpserver-and-readiness | 4 | CH07, CH03 | ✅ |
| CH11 | pipeline-metrics-and-alerts | 5 | CH10, CH07 | ✅ (O2,R1; O5 вынесена) |
| CH12 | log-correlation | 5 | CH10 | ✅ |
| CH13 | distributed-tracing | 5 | CH05, CH10 | ☐ |
| CH14 | bus-publish-perf | 6 | CH07 | ✅ |
| CH15 | ingestion-throughput | 6 | CH09, CH11 | ✅ |
| CH16 | tenantcache-singleflight | 6 | CH01 | ✅ |
| CH17 | test-hardening | 7 | CH01 | ✅ |
| CH18 | docs-and-style | 7 | CH01 | ☐ |
| CH19 | containerize-and-scan | 7 | CH01 | ☐ |

Прогресс: **16 / 19** done.

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
>   - **Локальный запуск (грабли, CH07):** `./tools` вне `go.work`, поэтому собирать/гонять инструменты надо с **`GOWORK=off`** — иначе `go -C tools build ...` падает с `package ... is not in std` / `directory prefix . does not contain modules listed in go.work`. Пример: `GOWORK=off go -C tools build -o /tmp/golangci-lint github.com/golangci/golangci-lint/v2/cmd/golangci-lint` (так же govulncheck).
>   - **golangci-lint гоняется ПОМОДУЛЬНО** (как в CI: `working-directory: <module>`), не из корня воркспейса — `./...` из корня даёт `directory prefix . does not contain modules listed in go.work`. В каждом задетом модуле: `cd <module> && GOWORK=off /tmp/golangci-lint run --config <АБСОЛЮТНЫЙ путь>/.golangci.yml --new-from-merge-base main ./...` (конфиг — абсолютным путём, относительный не найдётся из подкаталога). `govulncheck` — тоже помодульно.
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

### CH08 · `db-atomicity-and-state-transitions` 🔴 — ✅ done (2026-06-14)
**Корень:** нет оптимистичной конкуренции на переходах состояний.
**Закрывает:** D1 (`FOR UPDATE SKIP LOCKED` в транзакции **или** CAS-`UPDATE ... WHERE status/tier=expected` + проверка `RowsAffected`), D3 (тот же CAS для `PatchStatus`), D2 (транзакционный `withTx`-хелпер для составных записей incident/escalation), D5 (keyset-пагинация `(created_at, id)`), E4 (логировать ошибку `AppendHistory` вместо `_ =`), R2 (проверять err/nil в post-write чтениях хендлера).
**Содержимое:** store-слой incident/escalation + хендлеры.
**Зависит от:** CH01 (а на проде — лучше после CH07, чтобы requeue работал чисто). Изолировать.

> **Реализовано.** Чейндж `db-atomicity-and-state-transitions` (MODIFIED-дельты `incident-management`,
> `escalation-policies`). См. ADR-0016.
> Что важно для следующих сессий:
> - **Паттерн guarded-CAS + `withTx`** — новая норма для переходов состояний. В каждом store
>   добавлены приватный `withTx(ctx, fn func(pgx.Tx) error)` и узкий интерфейс `dbConn`
>   (`Exec`/`Query`/`QueryRow`), которому удовлетворяют и `*pgxpool.Pool`, и `pgx.Tx` — для
>   переиспользования SQL из пула и из транзакции. **Новые переходы стройте так же.**
> - **escalation (D1/D2):** `Store.AdvanceEscalationState(ctx, st, expectedTier, expectedStatus, hist)` —
>   guarded-UPDATE (`WHERE id AND current_tier=$exp AND status=$exp`) + опц. `AppendHistory` в
>   одной транзакции; `RowsAffected()==0` → `store.ErrConflict` (= `errs.ErrConflict`).
>   `AdvanceOrExhaust` захватывает строку CAS **до** публикации и тихо пропускает при конфликте
>   (нет двойной эскалации). `UpdateEscalationState` оставлен для `Stop` (не CAS — идемпотентен).
> - **incident (D2/D3):** `Store.TransitionStatus(ctx, …, status, expectedStatus, authorID, hist)` —
>   guarded-UPDATE статуса + история в транзакции; конфликт → **HTTP 409** в `PatchStatus`.
>   `Store.CreateIncidentTx(ctx, inc, labels, hist, ia)` — атомарное создание инцидента из алерта
>   (consumer). **Сигнатуры интерфейсов handler/consumer изменены** (`UpdateStatus`→`TransitionStatus`,
>   `CreateIncident`+`MergeLabels`→`CreateIncidentTx`) — учесть в **CH10** (переразводка main/handler)
>   и **CH15** (ingestion-throughput, если затронет incident store).
> - **D5 keyset-пагинация:** `ListIncidents` сортирует `created_at DESC, id DESC`, курсор —
>   **непрозрачный base64-токен `(created_at|id)`** (`encodeCursor`/`decodeCursor`), не «голый» id.
>   Нераспознанный курсор → первая страница. **Не BREAKING** для API; курсоры «в полёте» между
>   деплоями перезапросят список.
> - **E4:** `AppendHistory`-ошибки больше не глушатся: вошедшие в транзакции откатывают её;
>   остальные (Stop, triggerTier, авто-резолв, PutLabels, AddComment) — warn-лог. **R2:** post-write
>   чтения в escalation-хендлере (AttachPolicy/ManualEscalate) → 500 при ошибке вместо `null`.
> - **Схема БД и wire-формат событий НЕ менялись**, миграций нет. Публикация `incident.*`/`escalation.*`
>   теперь после commit транзакции.
> - **Backlog для CH17 (T4):** интеграционные регресс-тесты на Postgres (CAS-конфликт перехода,
>   `CreateIncidentTx`-атомарность, стабильность keyset при равных `created_at`) — здесь покрыто
>   юнитами (escalator-skip, handler-409, cursor round-trip), live-Postgres-тесты отложены.
> - Проверки: `go build/vet/test` (incident, escalation), **`-race`** (чисто), `golangci-lint
>   --new-from-merge-base main` (0 new), `govulncheck` (0 достижимых), `go mod tidy` без диффа.
>   Предсуществующие `go vet` httpresponse-замечания в `*/handler_test.go` — backlog T5 (CH17).

### CH09 · `store-layering-and-pool-config` 🟡 — ✅ done (2026-06-14)
**Корень:** слой персистентности в `cmd/` + ненастроенный пул.
**Закрывает:** F2 (вынести `pgStore`/redis-адаптеры/SQL из `ingestion/cmd/server/main.go` в `internal/`), D4 (конфиг пула `MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime` в `pkg/db`).
**Зависит от:** CH01.

> **Реализовано.** Чейндж `store-layering-and-pool-config` (no-delta infra/layering, архив с `--skip-specs`).
> Что важно для следующих сессий:
> - **D4 — пул сконфигурирован в `pkg/db`:** `NewPool(ctx, dsn)` **сохранил сигнатуру** (5 `main.go` не тронуты),
>   после `ParseConfig` применяются дефолты `MaxConns=10/MinConns=2/MaxConnLifetime=30m/MaxConnIdleTime=5m`.
>   Override через env `DB_POOL_MAX_CONNS`/`DB_POOL_MIN_CONNS`/`DB_POOL_MAX_CONN_LIFETIME_SECONDS`/`DB_POOL_MAX_CONN_IDLE_TIME_SECONDS`
>   (читаются внутри `pkg/db` через `pkg/config`). Экспортируемые `PoolConfig`/`DefaultPoolConfig()` — юнит-тестируемы без БД.
>   **Для CH11** (`O2`): `pgxpool.Stat()` теперь имеет осмысленные границы пула для экспорта метрик.
> - **F2 — ingestion выровнен с остальными сервисами:** `pgStore`→`internal/store` (`store.New(pool)`),
>   `redisCacheAdapter`→`internal/dedup.RedisCache` (`dedup.NewRedisCache(rdb)`), `redisTokenStore`→новый
>   `internal/tokenstore` (`tokenstore.New(rdb)`). `package main` теперь только парсит конфиг и собирает зависимости.
>   Интерфейсы-потребители (`handler.Store`, `dedup.Cache`, `middleware.TokenStore`) не менялись.
> - **Поведение, API, события RabbitMQ, схема БД — без изменений; не BREAKING.** Дельты спека нет.
> - Проверки: `go build` всех 6 модулей, `go vet/test` (pkg, ingestion), `golangci-lint --new-from-merge-base main`
>   (0 new в pkg и ingestion), `go mod tidy` без диффа — чисто.

---

## Фаза 4 — Консолидация HTTP-сервера

### CH10 · `shared-httpserver-and-readiness` 🟡 — ✅ done (2026-06-14)
**Корень:** разнобой bootstrap/middleware между сервисами.
**Закрывает:** F4 (`pkg/httpserver` — сервер с едиными таймаутами + graceful shutdown), E1 (`Recoverer` во всех 5 сервисах), F10 (auth-toggle в общий хелпер), O6 (`RequestID` везде), S6 (входной rate-limit middleware), F5 (scheduling → `pkglogger.New(cfg.LogLevel)` + поле `LogLevel`), F9 (один `escalator.New`), O1 (content-aware `/readyz`: БД/Redis/AMQP + живость консьюмера), E6 (не отдавать `err.Error()` клиенту).
**Содержимое:** новый `pkg/httpserver`, переразводка всех `main.go`.
**Зависит от:** CH07 (сигнал живости консьюмера для `/readyz`), CH03 (auth-wiring собирается тут же).

> **Реализовано.** Чейндж `shared-httpserver-and-readiness` (no-delta infra/ops, архив с `--skip-specs`). См. ADR-0017.
> Что важно для следующих сессий:
> - **Новый пакет `pkg/httpserver`:** `Run(ctx, addr, handler, logger)` (единые таймауты Read/ReadHeader/Write=15s, Idle=60s + graceful shutdown, F4); `NewRouter(service, checks...)` — обязательная цепочка `RequestID → Recoverer → metrics` + `/healthz`(статический liveness) + content-aware `/readyz` + `/metrics` (E1/O6/O1); `RateLimit(rps, burst)` per-IP token bucket (S6); `Check{Name, Probe}` и `BoolCheck(name, ok)`. **Для CH11** (`O2`/`R1`): метрик-middleware и доменные метрики навешивать здесь; сейчас `metrics.Middleware` всё ещё метит `r.URL.Path` (R1 не трогался). **Для CH12** (`request_id`) и **CH13** (`otelhttp`): дополнять эту же цепочку.
> - **`/readyz` теперь content-aware (O1):** возвращает **503**, если любая критичная зависимость недоступна. Состав проверок задаёт каждый `main`: ingestion=Postgres+Redis+AMQP `conn.Ready`; incident=Postgres+AMQP+consumer; escalation/notification=Postgres(+Redis у notification)+consumer **только при заданном `RABBITMQ_URL`**; scheduling=Postgres(+Redis если поднялся). **Операционное изменение поведения k8s** (под помечается NotReady при сбое зависимости) — манифесты проб менять не нужно. `/healthz` остался статическим.
> - **Сигнал живости консьюмера — `pkg/amqp.Probe`** (atomic Up/Down), выставляется supervisor-петлёй `Consume` (`ConsumeOptions.Probe`); `Connection.Ready()` = соединение открыто. Каждый `Consumer` (incident/escalation/notification) держит probe и экспонирует `Healthy() bool`. Закрыт остаток из заметки CH07. Wire-формат не менялся.
> - **F10:** `pkgauth.MiddlewareOrPassthrough(opts, authDisabled, logger)` — единственная реализация fail-closed-toggle (ADR-0012 1:1); дубли из 4 `main.go` удалены. **F5:** scheduling перешёл на `pkglogger.New(cfg.LogLevel)` (+ поле `LogLevel`/env `LOG_LEVEL`). **F9:** escalation создаёт `escalator.New` один раз. **E6:** incident отдаёт стабильное `"invalid status transition"` (через `errors.As ErrInvalidTransition`), детали — в лог.
> - **S6:** ingestion ограничивает webhook-роуты per-IP (`RATE_LIMIT_RPS`=50/`RATE_LIMIT_BURST`=100 по умолчанию). In-memory **per-pod** — не делится между репликами.
> - **API/события RabbitMQ/схема БД — без изменений; не BREAKING.** Дельты спека нет.
> - Проверки: `go build/vet/test` всех 6 модулей, **`-race`** (pkg/amqp+httpserver, чисто), `golangci-lint --new-from-merge-base main` (0 new), `govulncheck` (0 достижимых), `go mod tidy` (`golang.org/x/time`→direct в pkg). Предсуществующие `go vet` httpresponse-замечания в `*/handler_test.go` — backlog T5 (CH17), не трогались.

---

## Фаза 5 — Наблюдаемость

### CH11 · `pipeline-metrics-and-alerts` 🟡 — ✅ done (2026-06-14, O2+R1; O5 вынесена)
**Корень:** конвейер слеп для метрик.
**Закрывает:** O2 (доменные + шинные метрики: alerts/incidents/escalations/notifications, ack/nack/requeue, длительность обработки, backlog, publish-ошибки, `pgxpool.Stat`), R1 (метить запрос `chi RoutePattern`, а не `r.URL.Path` — фикс кардинальности).
**Зависит от:** CH10 (метрик-middleware живёт там), CH07 (есть что мерить по консьюмеру).

> **Реализовано.** Чейндж `pipeline-metrics-and-alerts` (no-delta infra/observability, архив с `--skip-specs`). См. ADR-0018.
> Что важно для следующих сессий:
> - **R1:** `pkg/metrics.Middleware` метит `chi.RouteContext(r.Context()).RoutePattern()` вместо `r.URL.Path`; неузнанный путь → лейбл `"other"`. **Изменилось значение лейбла `path`** существующих `http_*`-метрик (имена метрик прежние) — будущие дашборды строить на шаблонах роутов.
> - **O2 шина — централизованно в `pkg/amqp`** (`pkg/amqp/metrics.go`): `amqp_messages_processed_total{queue,result=ack|requeue|drop}`, `amqp_message_processing_seconds{queue}` (в `Consume.process`), `amqp_publish_total{exchange,result=ok|error}` (в `Publisher.Publish`). **Новые консьюмеры/издатели получают сигналы бесплатно** — не дублировать в сервисах.
> - **O2 пул pgx:** `pkg/db.RegisterPoolMetrics(service, pool)` — коллектор поверх `pgxpool.Stat()` → `db_pool_*{service}`. Вызывается из всех 5 `main.go` после `NewPool`. **Для CH15** — есть метрики для замера throughput.
> - **O2 доменные** (паттерн `dedup`, package-level `var`+`init`): ingestion `ingestion_alerts_received_total{source}`; incident `incident_incidents_created_total`/`_resolved_total`; escalation `escalation_triggered/advanced/exhausted_total`, `escalation_getoncall_failures_total`, gauge `escalation_backlog` (монитор); notification `notification_sent_total{channel,result}`, `notification_rate_limited_total{channel}`. **Конвейерные метрики БЕЗ лейбла `tenant_id`** (кардинальность) — per-tenant разрез только в логах.
> - **O5 (scrape/alerts/дашборды) ВЫНЕСЕНА из CH11** по решению владельца — отдельный чейндж/бэклог. Метрики уже на `/metrics`, имена зафиксированы в ADR-0018; O5 их потребит (`deploy/k8s/monitoring/`: `ServiceMonitor`+`PrometheusRule`+дашборд).
> - **Лок lint pkg:** pkg в `go.work`, поэтому golangci-lint гонять **БЕЗ `GOWORK=off`** (с ним — `no go files to analyze`); сервисные модули — как раньше с `GOWORK=off`.
> - **API/события RabbitMQ/схема БД — без изменений; не BREAKING.** `go mod tidy`: prometheus → direct в incident/escalation/notification.
> - Проверки: `go build/vet/test` всех 6 модулей, `-race` (pkg+сервисные пакеты, чисто), `golangci-lint --new-from-merge-base main` (0 new), `govulncheck` (0 достижимых). Предсуществующие `go vet` httpresponse-замечания в `*/handler_test.go` — backlog T5 (CH17).

### CH12 · `log-correlation` 🟢 — ✅ done (2026-06-14)
**Корень:** логи без корреляции.
**Закрывает:** O4 (`slog.*Context` + инъекция `request_id`/`trace_id` в записи), E5 (единый ключ `"err"`).
**Зависит от:** CH10 (request_id middleware). Часть `trace_id` подключится после CH13.

> **Реализовано.** Чейндж `log-correlation` (no-delta infra/observability, архив с `--skip-specs`).
> Что важно для следующих сессий:
> - **O4 — context-aware slog-хендлер в `pkg/logger`:** `New(level)` оборачивает JSON-хендлер
>   в приватный `contextHandler`, который в `Handle(ctx, r)` достаёт `request_id` через
>   `chiMiddleware.GetReqID(ctx)` и добавляет атрибутом. **request_id попадает в запись только у
>   `*Context`-вызовов** (не-Context передают `context.Background()`). `WithAttrs`/`WithGroup`
>   переопределены, чтобы обёртка переживала `logger.With(...)`.
> - **Для CH13** (`distributed-tracing`): точка инъекции `trace_id`/`span_id` уже размечена
>   комментарием в `contextHandler.Handle` — добавить `trace.SpanContextFromContext(ctx)`, **call-sites
>   повторно трогать не нужно** (они уже на `*Context`). Фоновые пути (consumers/notifier/escalator/
>   monitor/`pkg/amqp`) переведены на `*Context` заранее именно ради trace_id — request_id там не
>   присутствует (background, не HTTP-scope).
> - **Перевод call-sites:** все логи HTTP-хендлеров 5 сервисов → `*Context(r.Context(), …)`; фоновые
>   пути → `*Context(ctx/runCtx, …)`. **Не на `*Context` оставлены** стартовые логи `main.go`,
>   `pkg/auth.MiddlewareOrPassthrough` (config-time), `pkg/migrate` (startup) и `notifier.incidentLink`
>   (нет ctx в сигнатуре) — там корреляция бессмысленна. В `pkg/amqp.process` для panic-recover и
>   невалидного конверта используется `runCtx` (drain-ctx ещё не создан/паника), для handler-веток — `ctx`.
> - **E5 — закреплено линтером:** ключ ошибки в slog исторически свёлся к `"err"` ещё в CH05–CH11
>   (0 slog-`"error"` к старту CH12); добавлен `sloglint` с `forbidden-keys: ["error"]` в `.golangci.yml`,
>   чтобы `"error"`-ключ не вернулся. **JSON-тела ответов `{"error": ...}` — контракт API, sloglint их
>   не трогает.** Дефолтный `no-mixed-args` тоже включился — учесть в новых slog-вызовах (не мешать
>   key-value и `slog.Attr` в одном вызове).
> - **API/события RabbitMQ/схема БД — без изменений; не BREAKING.** В JSON-логах request-scope
>   добавилось поле `request_id`. Дельты спека нет.
> - Проверки: `go build/vet/test` всех 6 модулей, **`-race`** (`pkg/logger`+`pkg/amqp`, чисто),
>   `golangci-lint --new-from-merge-base main` (0 new в pkg и 5 сервисах), `govulncheck` (0 достижимых),
>   `go mod tidy` без диффа (chi уже был direct в pkg). Предсуществующие `go vet` httpresponse-замечания
>   в `*/handler_test.go` — backlog T5 (CH17), не трогались.

### CH13 · `distributed-tracing` 🟡
**Корень:** нет сквозной трассировки.
**Закрывает:** O3 (OpenTelemetry: `otelhttp`, span'ы на store/клиенты, **проброс trace-context через `pkg/amqp.Envelope`**).
**Зависит от:** CH05 (конверт), CH10. Крупный — отдельным чейнджем, можно позже.

---

## Фаза 6 — Производительность (после метрик/бенчей)

### CH14 · `bus-publish-perf` 🟡 — ✅ done (2026-06-15)
**Корень:** канал AMQP пересоздаётся на каждую публикацию.
**Закрывает:** P1 (переиспользуемый канал/пул на `Publisher`).
**Зависит от:** CH07 (там уже переработан `pkg/amqp`), CH01 (бенчмарки). Требует benchstat до/после в коммите.

> **Реализовано.** Чейндж `bus-publish-perf` (no-delta perf, архив с `--skip-specs`; ADR не вводился — локальная оптимизация в рамках транспорта из ADR-0015).
> Что важно для следующих сессий:
> - **P1 — `pkg/amqp.Publisher` держит один долгоживущий канал** (`mu sync.Mutex` + `ch *amqp.Channel`,
>   `pkg/amqp/amqp.go`): приватный `channel(ctx)` переиспользует/лениво переоткрывает канал
>   (`IsClosed()`-проверка переживает реконнект `Connection`), `resetChannel()` сбрасывает его при
>   ошибке публикации (ретрай `publishWithRetry` переоткроет). Убраны `channel.open`/`channel.close`
>   (2 round-trip'а) с каждой публикации. Ретраи и метрика `amqp_publish_total` сохранены.
> - **amqp091-каналы не потокобезопасны для публикации** → весь `publish` под `mu` (сериализация
>   публикаций одного `Publisher`). Удержание мьютекса = один `PublishWithContext` (без confirms).
>   **Publisher confirms — вне объёма P1** (вернули бы round-trip), в бэклог. Пул каналов отклонён
>   до появления профиля, где сериализация станет узким местом.
> - **`Publisher.Close()`** разведён в graceful-shutdown ingestion/incident/escalation (перед
>   `amqpConn.Close()`). notification/scheduling не публикуют — не затронуты.
> - **benchstat (n=10, локальный docker-compose RabbitMQ):** `917µs → 22µs/op (−97.6%)`,
>   `2400 → 904 B/op (−62%)`, `73 → 29 allocs/op (−60%)`. Бенч `BenchmarkPublish`
>   (`pkg/amqp/amqp_bench_test.go`) требует live-брокер (`RABBITMQ_URL`, по умолчанию docker-compose),
>   `b.Skip` без него — в CI без брокера скипается. **Для CH15** (`ingestion-throughput`): издатель
>   теперь на переиспользуемом канале — группировку публикаций строить на нём.
> - **Wire-формат `Envelope`/payload, API, схема БД не менялись — не BREAKING.** Дельты спека нет.
> - Проверки: `go build/vet/test` всех 6 модулей, **`-race`** (`pkg/amqp`, чисто), `golangci-lint
>   --new-from-merge-base main` (0 new в pkg и 3 сервисах), `govulncheck` (0 достижимых),
>   `go mod tidy` без диффа. Предсуществующие `go vet` httpresponse-замечания в `*/handler_test.go` —
>   backlog T5 (CH17), не трогались.

### CH15 · `ingestion-throughput` 🟡 — ✅ done (2026-06-16)
**Корень:** последовательный I/O без батчинга.
**Закрывает:** P2 (батч-INSERT `raw_alerts` + пайплайн Redis + воркер-пул), P3 (multi-row INSERT в `MergeLabels`), P5 (один `json.Marshal` для `Alert`).
**Зависит от:** CH09 (store вынесен), CH11 (метрики для замера), CH01 (бенчи).

> **Реализовано.** Чейндж `ingestion-throughput` (no-delta perf, архив с `--skip-specs`; ADR не вводился — локальные оптимизации round-trip'ов в рамках существующего транспорта/хранилища).
> Что важно для следующих сессий:
> - **P2(а) ingestion — групповой `processAlerts`:** тело вебхука обрабатывается фазами Marshal(один раз) → `Deduplicator.Classify` (пайплайн Redis в один round-trip) → `Store.SaveRawAlerts` (`pgx.Batch`, один round-trip) → публикация неподавленных на переиспользуемом канале (CH14). Порядок алертов, семантика дедупа (в т.ч. дубль fingerprint **внутри** одного тела) и откат дедуп-ключа при ошибке публикации сохранены; ответ `503` при ошибке. **P2(б) воркер-пул консьюмеров — уже в CH07/C8**, здесь не трогался.
> - **Интерфейсы ingestion сменили сигнатуру** (не wire/API): `handler.Store.SaveRawAlert`→`SaveRawAlerts([]store.RawAlert)`; `handler.Publisher`→`PublishAlertPayload(ctx, tenantID, json.RawMessage)`; `dedup.Cache`+`Apply(...)`, `Deduplicator.Classify(...)` (старые `SetNX`/`Del`/`IsDuplicate`/`Clear` оставлены для отката/регрессий). **Для CH16** (`tenantcache-singleflight`): дедуп-пайплайн — отдельный путь, не пересекается.
> - **P3 incident — `mergeLabels` один multi-row upsert** через `unnest($2::text[], $3::text[]) ON CONFLICT DO UPDATE`; общий приватный helper покрывает и `MergeLabels`, и `CreateIncidentTx` (работает внутри транзакции CH08).
> - **P5 — единый `json.Marshal(Alert)`:** готовый `json.RawMessage` переиспользуется для `raw_alerts.payload` и для конверта (`Wrap(json.RawMessage)` не реэнкодит структуру) — `pkg/amqp` не трогался.
> - **benchstat (n=10, docker-compose):** raw_alerts batch −92.8% времени; mergeLabels unnest −89.3%; dedup pipeline −48.2%; marshal ~ по времени (I/O-bound, как и предсказывал аудит), но −28% B/op / −39% allocs. Файл `benchstat.txt` в архиве чейнджа. Бенчи I/O — `b.Skip` без живой инфры (паттерн CH14).
> - **Wire-формат `alert.received`, API, схема БД не менялись — не BREAKING.** Дельты спека нет.
> - Проверки: `go build/vet/test` (ingestion, incident), **`-race`** (ingestion handler/dedup, incident store — чисто), `golangci-lint --new-from-merge-base main` (0 new в обоих модулях), `govulncheck` (0 достижимых), `go mod tidy` без диффа. Предсуществующие `go vet` httpresponse-замечания в `incident/.../handler_test.go` — backlog T5 (CH17), не трогались.

### CH16 · `tenantcache-singleflight` 🟢 — ✅ done (2026-06-16)
**Корень:** stampede + неограниченный рост кэша.
**Закрывает:** C7 (`singleflight` вокруг fetch + вытеснение протухших ключей).
**Зависит от:** CH01. Маленький, самостоятельный.

> **Реализовано.** Чейндж `tenantcache-singleflight` (no-delta perf/correctness, архив с `--skip-specs`; ADR не вводился — локальная оптимизация внутреннего кэша notification).
> Что важно для следующих сессий:
> - **C7.1 stampede:** `tenantcache.Cache` обёрнут в `golang.org/x/sync/singleflight.Group` (`services/notification/internal/tenantcache/cache.go`) — одновременные промахи по одному `tenantSlug` схлопываются в **один** `GetTenantNotificationConfig` к scheduling, результат раздаётся всем ожидающим. Лок по-прежнему не держится через I/O (запись в `data` — под `mu` уже после fetch).
> - **C7.2 рост памяти:** фоновый sweeper (`runSweeper`/`evictExpired`, тикер = ttl) вычищает протухшие ключи; запускается из `New(ctx, fetcher, ttl)` и **останавливается по `ctx`**. **Сигнатура `New` сменилась** на `New(ctx, …)` — пробрасывается рабочий `ctx` сервиса из `services/notification/cmd/server/main.go` (graceful-stop чистки). Это внутренний пакет notification, не `pkg/*` — другие сервисы не затронуты.
> - **Семантика `Get` сохранена:** кеш-хит в пределах TTL без нового fetch; ошибка fetch не кешируется. Новый `cache_test.go` (white-box): дедупликация 20 параллельных промахов → 1 fetch, вытеснение, регрессы — прогнан под **`-race`**.
> - **API, события RabbitMQ, схема БД — без изменений; не BREAKING.** Дельты спека нет (спеки `notification-dispatch` кэш не описывают).
> - **CH17 (T4):** `tenantcache` теперь покрыт юнитами (закрыт пробел из `docs/audit/08-testing.md`) — но `goleak` (T3) на sweeper-горутину можно добавить в CH17.
> - Проверки: `go build/vet/test` (notification), **`-race`** (tenantcache, чисто), `golangci-lint --new-from-merge-base main` (0 new), `govulncheck` (0 достижимых), `go mod tidy` без диффа (`x/sync` уже direct).

---

## Фаза 7 — Тест/стиль-гигиена (низкий приоритет, многое уже энфорсит линтер)

### CH17 · `test-hardening` 🟢 — ✅ done (2026-06-16)
**Закрывает:** T3 (`goleak` в `consumer`/`monitor`/`pkg/amqp`), T4 (юниты на `pkg/amqp.Envelope`, `tenantcache`, store эскалации — регресс-гард к D1), T5 (`t.Parallel()` + table-driven для парсера/матрицы).
**Зависит от:** CH01 (CI их гоняет), CH07/CH08 (тесты на новые гарантии).

> **Реализовано.** Чейндж `test-hardening` (no-delta infra/tests, архив с `--skip-specs`; ADR не вводился). Только тестовый код + dev-зависимость `go.uber.org/goleak v1.3.0` в `pkg`/`incident`/`escalation`/`notification`. Продакшн-код, API, события RabbitMQ, схема БД — без изменений.
> Что важно для следующих сессий:
> - **T3 goleak** — `TestMain` с `goleak.VerifyTestMain(m)` в `pkg/amqp`, консьюмерах incident/escalation/notification и escalation `monitor`; в `tenantcache` sweeper-горутина теперь покрыта goleak (хвост CH16) — тесты с фоновыми горутинами обязаны отдавать **отменяемый ctx** (хелпер `newTestCache` с `t.Cleanup(cancel)`), иначе VerifyTestMain падает. **Новые пакеты с горутинами — добавлять такой же `TestMain`.**
> - **T4** покрыты все 6 целей: `pkg/amqp/envelope_test.go` (контракт Wrap/Unwrap); `escalation/monitor` (юнит `step()` на интерфейсном фейке, white-box); notification `ratelimit`/`dispatcher` (отменяемый по ctx backoff — регресс C5; dispatcher white-box со stub-RoundTripper, т.к. SSRF-guard блокирует loopback httptest); `scheduling/keycloak` (Admin API через `httptest`, клиент на `NewStdClient` без guard'а — loopback ОК).
>   - **`escalation/store` — настоящий integration-тест** (`//go:build integration`, `store_integration_test.go`): регресс **D1** — 16 параллельных `AdvanceEscalationState` на одной строке → ровно 1 победитель, остальные `ErrConflict` (под `-race`); + `ListExpiredStates` фильтр status/time. Миграции применяются идемпотентно (`pkgmigrate.Run("file://../../migrations", …)`), `t.Skip` без `DB_DSN`. **Грабли:** пул закрывать через `t.Cleanup(pool.Close)`, не `defer` в тесте — иначе DELETE-cleanup сидов идёт по закрытому пулу (defer срабатывает раньше Cleanup).
> - **T5** — `ParseISO8601Duration` (rotation) и матрица переходов стейт-машины incident переведены на table-driven с `t.Run`+`t.Parallel`; `t.Parallel()` добавлен в независимые юниты; **go vet httpresponse-nil-deref** в `handler_test.go` (incident/escalation/scheduling) починен DRY-хелперами `mustGet/mustPost/mustDo` (проверяют err до использования resp). **Закрыт backlog T5 из CH01–CH16.**
>   - **paralleltest** (был в baseline CH01) теперь активен на изменённых файлах. Тесты с общим состоянием (package-global gauge `backlog` в monitor; живой Postgres в store-integration) помечены `//nolint:paralleltest` с причиной — серийность осознанная.
>   - **N1 (package-comment для `dispatcher`)** всплыл на новом тест-файле — подавлен `//nolint:revive` с пометкой на **CH18** (не расширяли объём).
> - Проверки: `go build/vet/test` всех 5 модулей (зелено), **`-race`** (amqp/консьюмеры/monitor/tenantcache/dispatcher — чисто), `go test -tags integration` escalation store (живой Postgres — зелено, идемпотентная очистка), `golangci-lint --new-from-merge-base main` (0 new), `govulncheck` (0 достижимых), `go mod tidy` (только goleak добавлен).

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
| CH11 | O2, R1 (O5 вынесена в отдельный чейндж) |
| CH12 | O4, E5 |
| CH13 | O3 |
| CH14 | P1 |
| CH15 | P2, P3, P5 |
| CH16 | C7 |
| CH17 | T3, T4, T5 |
| CH18 | N1, N2, N3, N4, N5 |
| CH19 | DC5 |

Все находки 11 областей покрыты. 19 чейнджей; ~9 параллелятся сразу после CH01.
