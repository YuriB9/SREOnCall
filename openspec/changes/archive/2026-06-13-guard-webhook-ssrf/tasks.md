# Tasks — guard-webhook-ssrf

## 1. Общий барьер `pkg/ssrf`
- [x] 1.1 S2 — `pkg/ssrf/ssrf.go`: sentinel-ошибки `ErrInvalidURL`/`ErrNotHTTPS`/`ErrBlockedAddress`,
      предикат `isBlockedIP` (loopback/private/link-local/unspecified/multicast/ULA), `ValidateURL`
      (https + резолв + блок диапазонов), `GuardedDialContext` (блок IP на дозвоне).
- [x] 1.2 S2 — `pkg/ssrf/ssrf_test.go`: таблица — публичный https → ok; http → `ErrNotHTTPS`;
      `169.254.169.254`/`localhost`/`10.x`/`127.0.0.1`/`::1` → `ErrBlockedAddress`; битый URL →
      `ErrInvalidURL`. `t.Parallel()` + table-driven.

## 2. Сервис scheduling — валидация на записи (S2)
- [x] 2.1 S2 — `services/scheduling/internal/handler/handler.go:705-707`
      (`PutTenantNotificationConfig`): при непустом `mattermost_webhook_url` → `ssrf.ValidateURL`,
      ошибка → `writeError(w, 422, ...)`; пустой URL — прежнее поведение (сохранить текущее значение).
- [x] 2.2 S2 — обновить тесты scheduling-хендлера: PUT с приватным/http URL → 422; PUT с публичным
      https URL → 200; PUT с пустым URL не затирает и не валидирует.
- [x] 2.3 `go mod tidy` в модуле `services/scheduling` (новая зависимость на `pkg/ssrf`).

## 3. Сервис notification — фильтр на отправке (S2, defense in depth)
- [x] 3.1 S2 — `services/notification/internal/dispatcher/mattermost.go:17`
      (`NewMattermost`): `http.Client` с `Transport`, использующим `ssrf.GuardedDialContext`.
- [x] 3.2 `go mod tidy` в модуле `services/notification` (новая зависимость на `pkg/ssrf`).

## 4. Спецификации и ADR
- [x] 4.1 Дельта `specs/tenant-management/spec.md`: MODIFIED «Конфигурация уведомлений тенанта» —
      требование валидации URL на PUT + сценарии (reject http, reject приватного, accept https).
- [x] 4.2 Дельта `specs/notification-dispatch/spec.md`: MODIFIED «Отправка уведомлений в Mattermost
      через входящий вебхук» — блок приватных/не-https целей на отправке + сценарий.
- [x] 4.3 `docs/adr/0013-guard-webhook-ssrf.md`: блок приватных диапазонов + dial-time guard vs
      allowlist; барьер на двух границах.

## 5. Верификация
- [x] 5.1 `go build ./...`, `go vet ./...`, `go test ./...` во всех затронутых модулях (`pkg`,
      `services/scheduling`, `services/notification`).
- [x] 5.2 `golangci-lint run` (после CH01 — гейт; 0 new issues на затронутых файлах).
- [x] 5.3 `govulncheck ./...` — новых достижимых адвизори нет.
- [x] 5.4 `/opsx:verify`.

## 6. Финал
- [x] 6.1 Обновить `docs/spec-vs-code-audit.md` (закрыт пункт S2), если релевантно.
- [x] 6.2 Обновить статус CH04 в `docs/audit/00-roadmap.md` (дашборд + строка чейнджа).
