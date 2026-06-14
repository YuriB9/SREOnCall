## 1. pkg/metrics — R1: кардинальность HTTP-метрик

- [x] 1.1 R1 — заменить `r.URL.Path` на `chi.RouteContext(r.Context()).RoutePattern()` в обоих местах (`requestDuration`, `requestsTotal`), читать шаблон после `next.ServeHTTP`; пустой шаблон → лейбл `"other"`. `pkg/metrics/metrics.go:38,41`
- [x] 1.2 R1 — обновить/добавить юнит-тест: запрос на смонтированный chi-роут `/x/{id}` метит `path="/x/{id}"`, несматченный → `"other"`. `pkg/metrics/metrics_test.go`

## 2. pkg/amqp — O2: шинные золотые сигналы (централизованно)

- [x] 2.1 O2 — объявить метрики шины + `init()` `MustRegister` с PromQL-комментарием: `amqp_messages_processed_total{queue,result}`, `amqp_message_processing_seconds{queue}`, `amqp_publish_total{exchange,result}`. `pkg/amqp/metrics.go` (новый)
- [x] 2.2 O2 — инструментировать `Consume.process`: таймер длительности по `queue`; инкремент `amqp_messages_processed_total` с `result=ack|requeue|drop` в соответствующих ветках (включая drop при панике/невалидном конверте). `pkg/amqp/consume.go:163-195`
- [x] 2.3 O2 — инструментировать `Publisher.Publish`: `amqp_publish_total{exchange,result=ok|error}` по итогу (после ретраев). `pkg/amqp/amqp.go:127-144`
- [x] 2.4 O2 — юнит-тест на учёт ack/drop/requeue в `process` (через фейковую delivery) и publish ok/error. `pkg/amqp/metrics_test.go` (новый)

## 3. pkg/db — O2: метрики пула pgx (D4-хвост)

- [x] 3.1 O2 — `RegisterPoolMetrics(service string, pool *pgxpool.Pool)`: `prometheus.Collector` поверх `pool.Stat()` → `db_pool_*{service}` (acquired/idle/total/max/acquire-count/acquire-wait), с PromQL-комментарием по насыщению. `pkg/db/metrics.go` (новый)
- [x] 3.2 O2 — юнит-тест: коллектор отдаёт ожидаемые серии (без реальной БД, можно на закрытом/пустом пуле или фейке Stat). `pkg/db/metrics_test.go` (новый)

## 4. ingestion — O2: доменные метрики приёма

- [x] 4.1 O2 — `ingestion_alerts_received_total{source}` + `init()` с PromQL-комментарием; инкремент на успешно нормализованный алерт. `services/ingestion/internal/handler`
- [x] 4.2 O2 — вызвать `pkgdb.RegisterPoolMetrics("ingestion", pool)` после `NewPool`. `services/ingestion/cmd/server/main.go:35`

## 5. incident — O2: доменные метрики инцидентов

- [x] 5.1 O2 — `incident_incidents_created_total`, `incident_incidents_resolved_total` + `init()` с PromQL-комментарием; инкремент после `CreateIncidentTx` (created) и закрытия в `handleResolved` (resolved). `services/incident/internal/consumer/consumer.go:136,179`
- [x] 5.2 O2 — `pkgdb.RegisterPoolMetrics("incident", pool)`. `services/incident/cmd/server/main.go:34`

## 6. escalation — O2: метрики эскалаций + backlog

- [x] 6.1 O2 — `escalation_triggered_total`, `escalation_advanced_total`, `escalation_exhausted_total`, `escalation_getoncall_failures_total` + `init()` с PromQL-комментарием; инкременты в `triggerTier` (triggered, getoncall_failures), `AdvanceOrExhaust` (advanced/exhausted после успешного CAS). `services/escalation/internal/escalator/escalator.go:128,146,222-235`
- [x] 6.2 O2 — gauge `escalation_backlog`: устанавливать длину пачки `ListExpiredStates` на каждом тике монитора. `services/escalation/internal/monitor/monitor.go`
- [x] 6.3 O2 — `pkgdb.RegisterPoolMetrics("escalation", pool)`. `services/escalation/cmd/server/main.go:39`

## 7. notification — O2: метрики доставки

- [x] 7.1 O2 — `notification_sent_total{channel,result}` (delivered|failed), `notification_rate_limited_total{channel}` + `init()` с PromQL-комментарием; инкременты в точках записи `NotificationLog` (`dispatchChannel`, rate-limit ветка, `NotifyExhausted`). `services/notification/internal/notifier/notifier.go:248-291,107-148`
- [x] 7.2 O2 — `pkgdb.RegisterPoolMetrics("notification", pool)`. `services/notification/cmd/server/main.go:40`

## 8. scheduling — только pool-метрики

- [x] 8.1 O2 — `pkgdb.RegisterPoolMetrics("scheduling", pool)` (R1 приходит автоматически через общий middleware). `services/scheduling/cmd/server/main.go:32`

## 9. Верификация

- [x] 9.1 `go build ./...`, `go vet ./...`, `go test ./...` во всех затронутых модулях (pkg, ingestion, incident, escalation, notification, scheduling).
- [x] 9.2 `golangci-lint run --new-from-merge-base main` помодульно (GOWORK=off, конфиг абс. путём) — 0 new issues; `govulncheck ./...` — 0 достижимых.
- [x] 9.3 Проверить `/metrics` руками (или тестом): новые серии присутствуют, нет паники двойной регистрации.
- [x] 9.4 `openspec validate pipeline-metrics-and-alerts` (если применимо) и `/opsx:verify`.
- [ ] 9.5 Обновить статус CH11 в `docs/audit/00-roadmap.md` (дашборд + строка чейнджа); отметить, что O5 вынесена и остаётся открытой. `docs/spec-vs-code-audit.md` не трогаем — capability-дельты нет.
