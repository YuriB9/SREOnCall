## 1. pkg/auth — ядро укрепления (S3, S4, S5)

- [x] 1.1 Ввести структуру `auth.Options{JWKSURL, AdminKey, Issuer, Audience, AllowInsecureJWKS}` и сменить сигнатуру на `Middleware(Options) (func(http.Handler) http.Handler, error)` — `pkg/auth/auth.go:93`
- [x] 1.2 S5 — проверять схему JWKS URL: только `https`, либо `http` при `AllowInsecureJWKS` (иначе вернуть error) — `pkg/auth/auth.go:90,94`
- [x] 1.3 S4 — передать опции в парсер: `jwt.WithValidMethods(["RS256"])` + `jwt.WithExpirationRequired()` всегда; `jwt.WithIssuer`/`jwt.WithAudience` — когда `Issuer`/`Audience` непусты — `pkg/auth/auth.go:114`
- [x] 1.4 S3 — заменить `==` на `subtle.ConstantTimeCompare([]byte(got), []byte(adminKey)) == 1` с проверкой непустоты — `pkg/auth/auth.go:102`
- [x] 1.5 Обновить docstring `Middleware`/`Options`

## 2. Юнит-тесты pkg/auth (регресс-гард S3/S4/S5)

- [x] 2.1 Тест constant-time admin-key: верный ключ → service-метод, неверный/пустой → 401 — `pkg/auth/auth_test.go`
- [x] 2.2 Тесты валидации JWT: токен без `exp`, чужой `aud`, чужой `iss`, не-RS256 (`alg`) → отклоняются (через тестовый JWKS/ключ)
- [x] 2.3 Тест S5: `http`-JWKS без флага → `Middleware` возвращает error; с `AllowInsecureJWKS=true` — ок

## 3. services/incident — fail-closed (S1)

- [x] 3.1 config: поля `AuthDisabled`(`AUTH_DISABLED`), `Issuer`(`KEYCLOAK_ISSUER`), `Audience`(`KEYCLOAK_AUDIENCE`), `AllowInsecureJWKS`(`AUTH_INSECURE`) — `services/incident/internal/config/config.go`
- [x] 3.2 wiring: пустой JWKS → `Error`+`os.Exit(1)`; `AUTH_DISABLED` → passthrough+`Warn`; иначе `Middleware(Options{...})` — `services/incident/cmd/server/main.go:75-85`

## 4. services/escalation — fail-closed (S1)

- [x] 4.1 config: те же 4 поля — `services/escalation/internal/config/config.go`
- [x] 4.2 wiring fail-closed + `Middleware(Options{...})` — `services/escalation/cmd/server/main.go:95-105`

## 5. services/notification — fail-closed (S1)

- [x] 5.1 config: те же 4 поля — `services/notification/internal/config/config.go`
- [x] 5.2 wiring fail-closed + `Middleware(Options{...})` — `services/notification/cmd/server/main.go:100-110`

## 6. services/scheduling — fail-closed (S1)

- [x] 6.1 config: те же 4 поля — `services/scheduling/internal/config/config.go`
- [x] 6.2 wiring fail-closed + `Middleware(Options{...})` — `services/scheduling/cmd/server/main.go:91-101`

## 7. Документация и деплой

- [x] 7.1 Создать ADR-0012 «Fail-closed аутентификация и строгая валидация JWT» — `docs/adr/0012-fail-closed-auth-and-strict-jwt.md`
- [x] 7.2 Добавить новые env в `deploy/k8s` configmap'ы 4 сервисов (`AUTH_INSECURE=true` для in-cluster http JWKS + закомментированные issuer/audience). docker-compose содержит только инфру — Go-сервисы там не запускаются, менять нечего; локальный запуск требует `AUTH_DISABLED=true` (зафиксировано в ADR-0012)
- [x] 7.3 N/A — `docs/spec-vs-code-audit.md` в репозитории отсутствует; S1/S3/S4/S5 относятся к код-аудиту (`docs/audit/05-security.md`), статус ведётся в `docs/audit/00-roadmap.md`

## 8. Верификация

- [x] 8.1 `go build ./...`, `go vet ./...`, `go test ./...` по всем модулям воркспейса
- [x] 8.2 `go test -race ./pkg/auth/...`
- [x] 8.3 `golangci-lint run` (затронутые файлы) и `govulncheck ./...` — чисто
- [x] 8.4 `/opsx:verify` → `/opsx:archive --skip-specs`
