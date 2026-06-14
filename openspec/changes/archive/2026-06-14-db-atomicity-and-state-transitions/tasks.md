# Tasks: db-atomicity-and-state-transitions

## 1. escalation — guarded-CAS + транзакция перехода (D1, D2)

- [x] 1.1 — D1/D2: добавить приватный `withTx(ctx, fn func(pgx.Tx) error) error` и узкий
  интерфейс `dbConn` (Exec/Query/QueryRow) в `services/escalation/internal/store/store.go`
  (по образцу `CreatePolicy`, store.go:28-57).
- [x] 1.2 — D1/D2: добавить `AdvanceEscalationState(ctx, st, expectedTier, expectedStatus, hist)`
  в `services/escalation/internal/store/store.go` — guarded-UPDATE
  `WHERE id=$ AND current_tier=$expected AND status=$expected` + `AppendHistory` (если hist!=nil)
  в одной транзакции; `RowsAffected()==0` → `errs.ErrConflict`. Добавить алиас
  `var ErrConflict = errs.ErrConflict`. (заменяет безусловный `UpdateEscalationState`+`AppendHistory`,
  store.go:249-263, escalator.go:107-116/131-135)
- [x] 1.3 — D1: переписать `AdvanceOrExhaust` в `services/escalation/internal/escalator/escalator.go:100-141`
  на CAS — захватить `prevTier := st.CurrentTier`, вызвать `AdvanceEscalationState(...)` до публикации;
  при `errors.Is(err, store.ErrConflict)` — debug-лог и `return nil` (тихий пропуск, без публикации).
- [x] 1.4 — D1: добавить `AdvanceEscalationState` в интерфейс `Store` эскалатора
  (escalator.go:17-27) и в мок-стор теста (escalator_test.go).

## 2. incident — guarded-CAS статуса + транзакция (D3, D2)

- [x] 2.1 — D2: добавить `withTx` + `dbConn` и приватные tx-варианты вставок
  (`createIncident`/`mergeLabels`/`appendHistory`/`attachAlert`/`loadLabels`/`updateStatusRow`)
  в `services/incident/internal/store/store.go`; публичные методы оставить обёртками над `s.db`.
- [x] 2.2 — D3/D2: добавить `TransitionStatus(ctx, tenantID, id, status, expectedStatus, authorID, hist)`
  в `services/incident/internal/store/store.go` — guarded-UPDATE статуса
  `WHERE id=$ AND tenant_id=$ AND status=$expected` (switch по статусу как в UpdateStatus,
  store.go:157-194) + `AppendHistory` в одной транзакции; нет строки → `errs.ErrConflict`.
- [x] 2.3 — D2: добавить `CreateIncidentTx(ctx, inc, labels, hist, ia)` в
  `services/incident/internal/store/store.go` — `createIncident`+`mergeLabels`+`appendHistory`+
  `attachAlert` в одной транзакции.
- [x] 2.4 — D3: переписать `PatchStatus` в `services/incident/internal/handler/handler.go:151-209`
  на `TransitionStatus` с `expectedStatus = inc.Status`; `errors.Is(err, store.ErrConflict)` →
  HTTP 409. История уходит в транзакцию (закрывает E4 handler.go:187).
- [x] 2.5 — D2: переписать `handleFiring` в `services/incident/internal/consumer/consumer.go:103-160`
  на `CreateIncidentTx` для ветки нового инцидента (Create+MergeLabels+History+AttachAlert);
  публикация `incident.created` — после commit. Закрывает E4 consumer.go:124.
- [x] 2.6 — D3/D2: обновить интерфейсы `Store` в handler (handler.go:23-40) и consumer
  (consumer.go:21-31) под новые методы; поправить мок-сторы в handler_test.go/consumer_test.go.

## 3. incident — keyset-пагинация (D5)

- [x] 3.1 — D5: в `ListIncidents` (`services/incident/internal/store/store.go:67-155`) заменить
  курсор по подзапросу `created_at < (SELECT … WHERE id=$)` (store.go:96-100) на keyset
  `WHERE (i.created_at, i.id) < ($cursorTime, $cursorID)` + `ORDER BY i.created_at DESC, i.id DESC`.
- [x] 3.2 — D5: ввести кодирование/декодирование непрозрачного курсора `(created_at, id)`
  (base64 `RFC3339Nano|id`); `next_cursor` — из последней строки страницы; нераспознанный
  курсор → первая страница (без 500).

## 4. error-handling / reliability (E4, R2)

- [x] 4.1 — E4: заменить оставшиеся `_ = AppendHistory` на Warn-лог в
  `services/escalation/internal/escalator/escalator.go:161` (Stop) и `:219` (triggerTier).
- [x] 4.2 — E4: заменить `_ = AppendHistory` на Warn-лог в
  `services/incident/internal/consumer/consumer.go:180` (авто-резолв) и
  `services/incident/internal/handler/handler.go:292` (PutLabels), `:348` (AddComment).
- [x] 4.3 — R2: в `services/escalation/internal/handler/handler.go:254` (AttachPolicy) и
  `:317` (ManualEscalate) проверять ошибку post-write чтения → Error-лог + HTTP 500 вместо
  `null`-тела.

## 5. Тесты

- [x] 5.1 — юнит escalator: конкурентный advance — при `store.ErrConflict` от
  `AdvanceEscalationState` событие НЕ публикуется, `AdvanceOrExhaust` возвращает nil
  (escalator_test.go).
- [x] 5.2 — юнит incident handler: `PatchStatus` при `store.ErrConflict` → 409; успешный
  переход → 200 + публикация (handler_test.go).
- [x] 5.3 — юнит incident store: декодирование/кодирование курсора и обработка нераспознанного
  курсора (первая страница).
- [~] 5.4 — ОТЛОЖЕНО в CH17 (T4): интеграционные (`-tags integration`, Postgres из
  docker-compose) регресс-тесты — guarded-CAS перехода статуса и advance эскалации возвращают
  конфликт при устаревшем состоянии; `CreateIncidentTx` атомарен; keyset стабилен при равных
  `created_at`. Логика покрыта юнитами (5.1–5.3); live-Postgres-тесты — отдельная работа CH17.

## 6. Верификация

- [x] 6.1 — `go build ./...`, `go vet ./...`, `go test ./...` в каждом задетом модуле
  (incident, escalation, pkg).
- [x] 6.2 — `go test -race ./...` (incident/escalation — конкурентные переходы).
- [~] 6.3 — `go test -tags integration ./...` — НЕ ПРОГОНЯЛОСЬ: в incident/escalation нет
  integration-тегнутых тестов на этих путях; новые отложены в CH17 (см. 5.4).
- [x] 6.4 — `golangci-lint run` помодульно (`--new-from-merge-base main`, GOWORK=off, конфиг
  абсолютным путём) — 0 new issues в задетых модулях.
- [x] 6.5 — `go mod tidy` в задетых модулях — без диффа.
- [x] 6.6 — `/opsx:verify` → `/opsx:archive`.

## 7. Документация

- [x] 7.1 — создать `docs/adr/0016-optimistic-state-transitions.md` (guarded-CAS + транзакции).
- [x] 7.2 — обновить статус CH08 в `docs/audit/00-roadmap.md` (дашборд + строка чейнджа) на
  `✅ done` с заметкой для следующих сессий (новые store-методы, формат курсора, паттерн CAS).
- [~] 7.3 — `docs/spec-vs-code-audit.md` отсутствует в репозитории (артефакт спек↔код-сверки не
  заведён); статус находок аудита ведётся в `docs/audit/00-roadmap.md` (см. 7.2). Нечего обновлять.
