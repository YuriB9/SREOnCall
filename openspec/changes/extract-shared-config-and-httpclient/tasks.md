# Tasks — extract-shared-config-and-httpclient (CH06: F3, F6, E3, P4)

Каждая задача привязана к находке аудита и `file:line`. Объём заморожен: только F3, F6, E3, P4.

## 1. Общие пакеты `pkg/*`

- [x] 1.1 **E3** — `pkg/errs`: `ErrNotFound`, `ErrConflict` (единственные значения на монорепо). Корень E3 — четыре разных `ErrNotFound`: `incident/store.go:17`, `escalation/store.go:13`, `scheduling/store.go:15`, `notification/store.go:12`.
- [x] 1.2 **F3+E3+P4** — `pkg/httpclient`: `Client{baseURL,adminKey}` с нормализацией `baseURL` (фикс дрейфа `escalation/schedclient/client.go:30` vs `notification/schedclient/client.go:31`), `GetJSON` с маппингом `404→errs.ErrNotFound`/`409→errs.ErrConflict` (E3, `incclient/client.go:48-50`, `schedclient/client.go:48-50`), общий тюнингованный `http.Transport` (P4, `client.go:33`) + `NewStdClient(timeout)` для не-admin-key клиентов.
- [x] 1.3 **F6** — `pkg/config`: `String`/`Int`/`DurationSeconds`. `Int` через `strconv.Atoi` — принимает легитимный `0` (фикс `notification/config.go:40-48`, где `Sscanf` отвергал `0`).

## 2. Сервис escalation (F3, E3, P4)

- [x] 2.1 `escalation/internal/schedclient/client.go` → обёртка над `pkg/httpclient` (только эндпойнт+DTO `OncallResult`).
- [x] 2.2 `escalation/internal/incclient/client.go` → обёртка над `pkg/httpclient`; 404→`errs.ErrNotFound` доступен вызывающему (E3, `incclient/client.go:48-50`).
- [x] 2.3 `escalation/internal/store/store.go:13` → `ErrNotFound` алиас на `errs.ErrNotFound`.
- [x] 2.4 `escalation/internal/config/config.go` → примитивы `pkg/config` (F6, `getenv:38`).

## 3. Сервис notification (F3, F6, E3, P4)

- [x] 3.1 `notification/internal/schedclient/client.go:31` → обёртка над `pkg/httpclient` (нормализация `baseURL` — фикс trailing slash); контракт «404 → nil без ошибки» через `errors.Is(err, errs.ErrNotFound)`.
- [x] 3.2 `notification/internal/store/store.go:12` → `ErrNotFound` алиас на `errs.ErrNotFound`.
- [x] 3.3 `notification/internal/config/config.go` → примитивы `pkg/config`; `getenvInt` (`:40-48`) заменён `config.Int` (фикс `0`).
- [x] 3.4 **P4** — `notification/internal/dispatcher/mattermost.go:21-24` → поднять `MaxIdleConnsPerHost` на клонированном транспорте, **сохранив** SSRF-guarded dialer из CH04.

## 4. Сервис scheduling (F6, E3, P4)

- [x] 4.1 `scheduling/internal/keycloak/client.go:29` → `httpClient` из `httpclient.NewStdClient` (общий тюнингованный Transport, P4).
- [x] 4.2 `scheduling/internal/store/store.go:14-15` → `ErrNotFound`/`ErrConflict` алиасы на `errs.*`; проверить `OverrideConflictError.Is` (`:30-32`).
- [x] 4.3 `scheduling/internal/config/config.go` → примитивы `pkg/config` (F6, `getenv:34`).

## 5. Сервисы incident, ingestion (F6, E3)

- [x] 5.1 `incident/internal/store/store.go:17` → `ErrNotFound` алиас на `errs.ErrNotFound`.
- [x] 5.2 `incident/internal/config/config.go` → примитивы `pkg/config` (F6, `getenv:25`).
- [x] 5.3 `ingestion/internal/config/config.go` → примитивы `pkg/config` (F6, `envOr:35`, `envDurSec:42`).

## 6. Верификация (Definition of Done)

- [x] 6.1 `go mod tidy` в каждом задетом модуле (go.work) — без диффа.
- [x] 6.2 `go build ./...` + `go vet ./...` всех модулей.
- [x] 6.3 `go test ./...` всех модулей (особенно existing handler-тесты на `errors.Is`).
- [x] 6.4 `golangci-lint run` (`--new-from-merge-base main`) — 0 new issues.
- [x] 6.5 `govulncheck ./...` — чисто.
- [x] 6.6 `/opsx:verify` → `/opsx:archive --skip-specs` (no-delta infra).
- [x] 6.7 Обновить статус CH06 в `docs/audit/00-roadmap.md` (дашборд + строка чейнджа).
