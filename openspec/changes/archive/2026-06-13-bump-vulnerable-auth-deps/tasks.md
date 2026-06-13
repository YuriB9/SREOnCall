## 1. Бамп зависимостей в pkg

- [x] 1.1 DC1 — `jwt/v5 v5.2.1 → v5.2.2` (GO-2025-3553): `go get github.com/golang-jwt/jwt/v5@v5.2.2` в `pkg/` ([pkg/go.mod:8](../../../pkg/go.mod#L8), достижимо из [pkg/auth/auth.go:114](../../../pkg/auth/auth.go#L114))
- [x] 1.2 DC1 — фикс GO-2025-3376: версия `jwkset v0.6.0` из аудита **ретрактнута** (PR #42), а текущая `keyfunc/v3 v3.3.5` сама в ретракт-диапазоне → бамп `keyfunc/v3 → v3.8.0` + `jwkset v0.5.19 → v0.11.0` в `pkg/` ([pkg/go.mod:18](../../../pkg/go.mod#L18), достижимо из [pkg/auth/auth.go:94](../../../pkg/auth/auth.go#L94))
- [x] 1.3 `go mod tidy` в `pkg/`; проверить `git diff` по `pkg/go.mod` и `pkg/go.sum`

## 2. Синхронизация транзитивных версий в сервисах

- [x] 2.1 DC1 — `go mod tidy` в `services/scheduling/` → `jwt/v5`/`jwkset`/`keyfunc/v3` подтянуты до фикса ([services/scheduling/go.mod](../../../services/scheduling/go.mod))
- [x] 2.2 DC1 — `go mod tidy` в `services/incident/` ([services/incident/go.mod](../../../services/incident/go.mod))
- [x] 2.3 DC1 — `go mod tidy` в `services/escalation/` ([services/escalation/go.mod](../../../services/escalation/go.mod))
- [x] 2.4 DC1 — `go mod tidy` в `services/notification/` ([services/notification/go.mod](../../../services/notification/go.mod))
- [x] 2.5 `services/ingestion` auth не использует — auth-зависимостей в дереве нет (подтверждено `go mod tidy` + grep)

## 3. Ужесточение CI-гейта (хэндофф CH01)

- [x] 3.1 DC1/CH01-хэндофф — снят `continue-on-error: true` с джоба `govulncheck` и убран «временно информационный» комментарий ([.github/workflows/ci.yml](../../../.github/workflows/ci.yml))

## 4. Верификация

- [x] 4.1 `go build ./...` + `go vet ./...` по каждому модулю — собираются (vet-замечания в `handler_test.go` escalation/incident/scheduling пре-существующие, T5/CH17, вне объёма)
- [x] 4.2 `go test ./...` по каждому модулю — регрессий нет
- [x] 4.3 `govulncheck ./...` в `pkg` и каждом сервисе — `0 vulnerabilities`, GO-2025-3553 и GO-2025-3376 отсутствуют
- [x] 4.4 `golangci-lint` (only-new-issues) — Go-исходники не менялись, новых замечаний нет
- [x] 4.5 `/opsx:verify` — критичных/warning-замечаний нет, готово к архивации
- [x] 4.6 Обновлён статус CH02 в [docs/audit/00-roadmap.md](../../../docs/audit/00-roadmap.md) (дашборд ✅ + строка чейнджа с хэндофф-заметкой); сверка спек↔код не требуется (нет изменения гарантий capability)
