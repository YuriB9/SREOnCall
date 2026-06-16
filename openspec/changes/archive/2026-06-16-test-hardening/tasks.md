## 1. Подготовка зависимости goleak (T3)

- [x] 1.1 Добавить `go.uber.org/goleak` как dev-зависимость в модули `pkg`, `services/incident`, `services/escalation`, `services/notification`; `go mod tidy` в каждом

## 2. T3 — детекция утечек горутин (goleak)

- [x] 2.1 T3 — `TestMain` с `goleak.VerifyTestMain(m)` в `pkg/amqp` (reconnect/Consume), pkg/amqp/main_test.go
- [x] 2.2 T3 — `TestMain` goleak в `services/incident/internal/consumer` (цикл Run), consumer/main_test.go
- [x] 2.3 T3 — `TestMain` goleak в `services/escalation/internal/consumer`, consumer/main_test.go
- [x] 2.4 T3 — `TestMain` goleak в `services/escalation/internal/monitor` (тикер), monitor/main_test.go
- [x] 2.5 T3 — `TestMain` goleak в `services/notification/internal/consumer`, consumer/main_test.go
- [x] 2.6 T3 — подключить реальный `goleak` на sweeper-горутину в `tenantcache` (хвост CH16), services/notification/internal/tenantcache/cache_test.go

## 3. T4 — недостающие юниты и integration-тесты

- [x] 3.1 T4 — юнит контракта конверта: round-trip Wrap/Unwrap, версия, ошибка на битом payload, битый/неизвестный payload, pkg/amqp/envelope.go → pkg/amqp/envelope_test.go
- [x] 3.2 T4 — integration-тест store эскалации (тег `integration`, Postgres, skip без DB_DSN): регресс D1 — CAS-конфликт AdvanceEscalationState под параллелизмом + ListExpiredStates SKIP LOCKED, services/escalation/internal/store/store.go:298,320 → store/store_integration_test.go
- [x] 3.3 T4 — юнит `monitor.step()` на мок-Store/escalator (без БД), services/escalation/internal/monitor/monitor.go:45 → monitor/monitor_test.go
- [x] 3.4 T4 — юнит token-bucket rate-limit (S6), services/notification/internal/ratelimit/ratelimit.go → ratelimit/ratelimit_test.go
- [x] 3.5 T4 — юнит dispatcher: отменяемое ожидание ретрая по ctx (регресс C5), services/notification/internal/dispatcher/{email,mattermost}.go → dispatcher/*_test.go
- [x] 3.6 T4 — юнит Keycloak Admin API клиента через httptest, services/scheduling/internal/keycloak/client.go → keycloak/client_test.go

## 4. T5 — параллелизм, table-driven, go vet

- [x] 4.1 T5 — `ParseISO8601Duration` → table-driven с `t.Run(name)` + `t.Parallel()`, services/scheduling/internal/rotation/rotation_test.go:25
- [x] 4.2 T5 — матрица переходов стейт-машины → именованные подтесты + `t.Parallel()`, services/incident/internal/statemachine/statemachine_test.go:10
- [x] 4.3 T5 — добавить `t.Parallel()` в независимые юнит-тесты, где его нет (в задетых пакетах)
- [x] 4.4 T5 — починить go vet httpresponse-nil-deref (resp до проверки err): services/incident/internal/handler/handler_test.go:321; services/escalation/internal/handler/handler_test.go:330,358; services/scheduling/internal/handler/handler_test.go (345,416,705,712,727,735,757,781,801,816,831,852)

## 5. Верификация

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` во всех затронутых модулях (httpresponse-замечания исчезли)
- [x] 5.2 `go test -race ./...` для `pkg/amqp`, консьюмеров, `monitor`, `tenantcache` — чисто (goleak не падает)
- [x] 5.3 Поднять Postgres из docker-compose, прогнать `go test -tags integration ./internal/store/...` в escalation — зелёно
- [x] 5.4 `golangci-lint run --new-from-merge-base main` (помодульно, включая `paralleltest`) — 0 new; `govulncheck ./...` — 0; `go mod tidy` без диффа (кроме добавленного goleak)
- [x] 5.5 Обновить статус CH17 в docs/audit/00-roadmap.md (дашборд + строка чейнджа); закрыть пункт в docs/spec-vs-code-audit.md, если применимо
