# Proposal: db-atomicity-and-state-transitions

## Why

Аудит работы с БД (`docs/audit/03-database.md`) показал, что базовая гигиена доступа к
Postgres хорошая (сплошная параметризация, `rows.Close()`/`rows.Err()`, проброс `ctx`), но
переходы состояний **не атомарны и не защищены от гонок** — это даёт двойную эскалацию,
обход стейт-машины инцидента и частичные записи. Сопутствующие находки из областей 4 и 6
закрывают тихую потерю аудит-трейла и `null`-ответы.

- **D1 (major).** `ListExpiredStates` использует `FOR UPDATE SKIP LOCKED` **вне транзакции**
  (`pool.Query`): блокировка снимается сразу по завершении SELECT, к моменту
  `AdvanceOrExhaust` строка уже не залочена. `UpdateEscalationState` пишет безусловно
  `WHERE id=$`. При двух репликах escalation/перекрытии тиков обе реплики прочитают один
  просроченный state и обе выполнят advance → **двойная эскалация и двойная публикация
  `escalation.triggered`** ([store.go:266-292](services/escalation/internal/store/store.go#L266-L292),
  [escalator.go:100-141](services/escalation/internal/escalator/escalator.go#L100-L141)).
- **D3 (major).** `PatchStatus` — классическая гонка check-then-act: `Validate` против
  прочитанного статуса, затем безусловный `UPDATE` без guard по старому статусу. Два
  параллельных PATCH (UI `acknowledge` + конвейерный `resolve`) оба проходят валидацию против
  одного `open` и оба пишутся (last-writer-wins, стейт-машина обойдена)
  ([handler.go:151-209](services/incident/internal/handler/handler.go#L151-L209),
  [store.go:157-194](services/incident/internal/store/store.go#L157-L194)).
- **D2 (major).** Многошаговые записи идут отдельными автокоммитами: создание инцидента
  (`CreateIncident`→`MergeLabels`→`AppendHistory`→`AttachAlert`), смена статуса
  (`UpdateStatus`+`AppendHistory`), advance эскалации (`Update`+`AppendHistory`). Ошибка/отмена
  между шагами оставляет частичное состояние ([consumer.go:103-160](services/incident/internal/consumer/consumer.go#L103-L160)).
- **D5 (minor).** Курсорная пагинация инцидентов по неуникальному `created_at`: пропуски/дубли
  на границах страниц при одинаковых таймстампах; несуществующий cursor-id даёт `NULL`-подзапрос
  и молча пустую страницу ([store.go:96-100](services/incident/internal/store/store.go#L96-L100)).
- **E4 (minor).** Ошибки `AppendHistory` глушатся через `_ =` в 8 местах — аудит-трейл молча
  теряется ([escalator.go:111,161,219](services/escalation/internal/escalator/escalator.go#L111),
  [consumer.go:124,180](services/incident/internal/consumer/consumer.go#L124),
  [handler.go:187,292,348](services/incident/internal/handler/handler.go#L187)).
- **R2 (minor).** Post-write чтения в escalation-хендлере игнорируют ошибку и nil
  (`st, _ := GetEscalationStateByIncident`) → клиент получает `201/200` с телом `null` вместо
  `5xx`, ошибка нигде не логируется ([handler.go:254,317](services/escalation/internal/handler/handler.go#L254)).

Корень D1/D3 — отсутствие оптимистичной конкуренции на переходах состояний; D2 — отсутствие
транзакционного обёртывания составных записей. Чейндж вводит единый паттерн **guarded-CAS +
`withTx`** для всех переходов и закрывает сопутствующие E4/R2. Детали — в [design.md](design.md),
ADR-0016.

## What Changes

- **escalation/store + escalator (D1, D2):** новый транзакционный метод
  `AdvanceEscalationState(ctx, st, expectedTier, expectedStatus, hist)` — внутри одной
  транзакции выполняет **guarded-UPDATE** (`WHERE id=$ AND current_tier=$expected AND status=$expected`)
  и `AppendHistory`. `RowsAffected()==0` → `errs.ErrConflict` (строку уже забрал другой
  обработчик). `AdvanceOrExhaust` захватывает строку этим CAS **до** публикации
  `escalation.triggered`/`escalation.exhausted`; при конфликте — тихо пропускает (debug-лог),
  не публикуя. `FOR UPDATE SKIP LOCKED` в `ListExpiredStates` остаётся как дешёвая
  разгрузка реплик, но корректность теперь обеспечивает CAS.
- **incident/store + handler (D3, D2):** новый транзакционный метод
  `TransitionStatus(ctx, …, status, expectedStatus, …, hist)` — guarded-UPDATE статуса
  (`WHERE id=$ AND tenant_id=$ AND status=$expected`) + `AppendHistory` в одной транзакции.
  `PatchStatus` передаёт прочитанный статус как `expectedStatus`; конфликт → **HTTP 409**.
- **incident/store + consumer (D2):** новый транзакционный метод
  `CreateIncidentTx(ctx, inc, labels, hist, ia)` — `CreateIncident`+`MergeLabels`+`AppendHistory`+
  `AttachAlert` в одной транзакции (атомарное создание инцидента из алерта; заодно закрывает
  риск частичной записи при отмене `ctx`/requeue из C3).
- **incident/store (D5):** keyset-пагинация по составному курсору `(created_at, id)`:
  `WHERE (i.created_at, i.id) < ($cursorTime, $cursorID) ORDER BY i.created_at DESC, i.id DESC`.
  Курсор теперь несёт обе компоненты (opaque-токен), подзапрос по id убран — нет ни пропусков
  на равных таймстампах, ни молчаливо пустой страницы при неизвестном id.
- **E4:** все `_ = AppendHistory` заменены: записи, вошедшие в новые транзакции
  (создание инцидента, advance, смена статуса), теперь возвращают ошибку и откатывают
  транзакцию; оставшиеся вне транзакций (Stop эскалации, triggerTier, авто-резолв,
  PutLabels, AddComment) — логируются на уровне Warn.
- **R2:** post-write чтения в `AttachPolicy`/`ManualEscalate` проверяют ошибку → Error-лог +
  HTTP 500 вместо `null`-тела.

**Изменения поведения** (без изменения wire-формата событий/схемы БД, без миграций):
параллельная смена статуса инцидента, конфликтующая с уже применённой, отклоняется `409`;
просроченное состояние эскалации продвигается ровно один раз даже при нескольких
репликах/перекрытии тиков; пагинация инцидентов стабильна на равных таймстампах. Формат
курсора пагинации меняется (непрозрачный токен) — **не BREAKING** для API-схемы, влияет только
на курсоры «в полёте» между деплоями.

## Impact

- **Затронутые сервисы:** incident (store, handler, consumer), escalation (store, escalator,
  handler). scheduling/notification/ingestion не затронуты. Изменения только во внутренних
  методах store — публичных HTTP-контрактов не ломают (409 уже задокументирован в `config.yaml`
  как код конфликта).
- **События RabbitMQ:** `escalation.triggered`/`escalation.exhausted` (publish: escalation) —
  **wire-формат НЕ меняется**; меняется лишь гарантия «ровно один раз на просроченное
  состояние» (раньше при гонке публиковались дубликаты). `incident.created`/`incident.updated`
  (publish: incident) — публикация теперь **после** успешного commit транзакции; формат и
  набор полей не меняются. **Не BREAKING.**
- **Схема БД:** изменений схемы и миграций НЕТ — только новые/изменённые запросы (guarded
  `WHERE`, keyset-пагинация, `BEGIN/COMMIT`).
- **Capability-спеки (MODIFIED):**
  - `escalation-policies` — «Срабатывание эскалации по таймауту»: добавлена гарантия, что
    просроченное состояние продвигается ровно один раз при конкурентных обработчиках (нет
    двойной эскалации).
  - `incident-management` — «Управление жизненным циклом инцидента»: конкурирующая/устаревшая
    смена статуса отклоняется `409`; «Список инцидентов»: пагинация keyset по `(created_at, id)`
    стабильна при равных таймстампах.
- **ADR:** ADR-0016 — оптимистичная конкуренция на переходах состояний (guarded-CAS) и
  транзакционные составные записи (значимое решение по хранилищу/pgx).
- **Зависимости:** CH01 (CI-гейт, заархивирован и влит). На проде эффективнее после CH07
  (requeue работает чисто) — CH07 уже влит. Изолированный чейндж (🔴).
- **Внешнее поведение API/UI:** PATCH статуса при конфликте отдаёт `409` (новый наблюдаемый
  ответ для гонки); прочие контракты без изменений.
