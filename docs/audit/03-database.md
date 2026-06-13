# Аудит SREOnCall — Область 3: Работа с БД (pgx)

Дата: 2026-06-13
Область: параметризация запросов, транзакции, пул соединений, обработка NULL, проброс контекста.
Применённый скил: `golang-database`.

Стек подтверждён: `pgx/v5` + `pgxpool`, миграции через `golang-migrate` (внешний инструмент — корректно), ORM нет, SQL пишется явно. Базовая гигиена доступа к БД **на хорошем уровне**: параметризация сплошная (SQL-инъекций не найдено), `rows.Close()`/`rows.Err()` присутствуют везде, NULL обрабатывается осознанно, `ctx` пробрасывается во все вызовы. Реальные проблемы лежат глубже — в **атомарности**: транзакции почти не используются, есть неэффективная блокировка строк и две гонки read-modify-write.

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| D1 | **major** | `FOR UPDATE SKIP LOCKED` вне транзакции — блокировка снимается сразу после SELECT, read-modify-write монитора не атомарен → двойная эскалация/двойное уведомление | [escalation/store.go:264-273](services/escalation/internal/store/store.go#L264-L273), [escalator.go:AdvanceOrExhaust](services/escalation/internal/escalator/escalator.go) |
| D2 | **major** | Многошаговые записи без транзакции: создание инцидента, смена статуса, advance эскалации — каждый набор INSERT/UPDATE идёт отдельными автокоммитами → частичное состояние при ошибке/отмене | [incident/consumer.go:148-178](services/incident/internal/consumer/consumer.go#L148-L178), [incident/handler.go:PatchStatus](services/incident/internal/handler/handler.go) |
| D3 | **major** | `PatchStatus`: check-then-act гонка — `Validate` против прочитанного статуса, затем безусловный `UPDATE` без guard по старому статусу → потерянное обновление и обход стейт-машины | [incident/handler.go:PatchStatus](services/incident/internal/handler/handler.go), [incident/store.go:155-191](services/incident/internal/store/store.go#L155-L191) |
| D4 | **minor** | Пул соединений не сконфигурирован: `pkg/db.NewPool` не задаёт `MaxConns`/`MinConns`/`MaxConnLifetime`/`MaxConnIdleTime` — полагается только на DSN | [pkg/db/db.go:11-25](pkg/db/db.go#L11-L25) |
| D5 | **minor** | Курсорная пагинация по неуникальному `created_at` → пропуски/дубли на границах страниц; несуществующий cursor id даёт `NULL`-подзапрос и молча пустой ответ | [incident/store.go:93-96](services/incident/internal/store/store.go#L93-L96) |

---

## Детализация

### D1 — `FOR UPDATE SKIP LOCKED` без транзакции не блокирует ничего — **major**

`ListExpiredStates` написан так, будто защищает строки от параллельных обработчиков:

```go
rows, err := s.db.Query(ctx,
    `SELECT ... FROM escalation.incident_escalation_states
     WHERE status='active' AND escalate_at <= now()
     ORDER BY escalate_at ASC
     LIMIT $1
     FOR UPDATE SKIP LOCKED`, limit)   // store.go:264-273
```

Но вызов идёт через `pool.Query` **без явной транзакции**. В этом режиме каждый statement выполняется в собственной автокоммит-транзакции, которая завершается, как только строки прочитаны и соединение возвращается в пул (`defer rows.Close()`). То есть `FOR UPDATE` удерживает блокировку лишь на доли секунды самого SELECT — к моменту, когда `monitor.step` начинает итерировать результат и вызывать `AdvanceOrExhaust`, **блокировки уже нет**.

Дальше `AdvanceOrExhaust` выполняет классический read-modify-write полностью вне блокировки: читает следующий tier (`GetTierByNumber`) и пишет новое состояние (`UpdateEscalationState`). А сам `UpdateEscalationState` обновляет строку **безусловно** по первичному ключу:

```sql
UPDATE ... SET current_tier=$1, status=$2, escalate_at=$3, updated_at=now()
WHERE id=$4              -- нет guard по старому tier/status
```

([store.go:UpdateEscalationState](services/escalation/internal/store/store.go)). Итог: при двух репликах escalation (или перекрытии тиков) обе прочитают один и тот же просроченный state и обе выполнят advance → **двойная эскалация и двойная публикация `escalation.triggered`** → дабл-нотификации дежурному. `SKIP LOCKED` создаёт ложное ощущение защиты, которой по факту нет.

**Фикс (два варианта из `golang-database`):**

1. **Транзакция на батч/строку.** Открыть `tx := db.Begin(ctx)`, внутри сделать `SELECT ... FOR UPDATE SKIP LOCKED`, обработать и `UPDATE`, затем `Commit` — тогда блокировка живёт весь read-modify-write.
2. **Optimistic CAS без транзакции.** Сделать UPDATE условным и «застолбить» работу через `RowsAffected`:

```sql
UPDATE ... SET current_tier=$new, escalate_at=$at, updated_at=now()
WHERE id=$id AND current_tier=$expectedOld AND status='active'
```
Если `RowsAffected()==0` — строку уже забрал другой обработчик, пропускаем. Это самый дешёвый фикс под текущую архитектуру монитора.

---

### D2 — Многошаговые записи не обёрнуты в транзакцию — **major**

Транзакция используется ровно в **одном** месте на весь код БД — `escalation.CreatePolicy` ([store.go:25-53](services/escalation/internal/store/store.go#L25-L53), сделано правильно: `Begin`/`defer Rollback`/`Commit`). Все остальные составные операции выполняются как независимые автокоммиты:

- **Создание инцидента** ([incident/consumer.go:148-178](services/incident/internal/consumer/consumer.go#L148-L178)): `CreateIncident` → `MergeLabels` → `AppendHistory` → `AttachAlert` — четыре отдельных стейтмента. Ошибка или отмена контекста (см. область 2, C3) между ними оставляет инцидент без labels/alert/истории, при этом `incident.created` либо уже опубликован, либо сообщение уйдёт в Nack+requeue и создаст дубликат.
- **Смена статуса** ([incident/handler.go:PatchStatus](services/incident/internal/handler/handler.go)): `UpdateStatus` и затем `AppendHistory` — раздельно; статус может смениться без записи в историю.
- **Advance эскалации** (`AdvanceOrExhaust`): `UpdateEscalationState` + `AppendHistory` + `PublishExhausted/triggerTier` — раздельно.

По скилу `golang-database` (п.7 «Use transactions for multi-statement operations») связанные записи должны идти одной транзакцией, чтобы либо применялись целиком, либо откатывались.

**Фикс.** Ввести в каждом `store` транзакционный хелпер (`func (s *Store) withTx(ctx, fn func(pgx.Tx) error) error` с `Begin`/`Rollback`/`Commit`) и завернуть в него составные операции. Для incident-консьюмера это заодно закрывает риск дубля из C3: при ошибке вся вставка откатывается, requeue переобрабатывает с чистого листа.

---

### D3 — `PatchStatus`: гонка check-then-act поверх стейт-машины — **major**

Хендлер валидирует переход против **прочитанного ранее** статуса, а пишет — безусловно:

```go
inc, _ := h.store.GetIncident(ctx, tenantID, id)          // читаем текущий статус
if err := statemachine.Validate(inc.Status, newStatus);   // валидируем переход
...
updated, _ := h.store.UpdateStatus(ctx, tenantID, id, newStatus, caller) // пишем без guard
```

`UpdateStatus` обновляет `WHERE id=$ AND tenant_id=$` — **без условия на старый статус** ([store.go:155-191](services/incident/internal/store/store.go#L155-L191)). Между `GetIncident` и `UpdateStatus` статус мог измениться другим запросом или consumer'ом (авторезолв по `MaybeResolve`). Два параллельных PATCH (например, `acknowledge` из UI и `resolve` из конвейера) оба пройдут `Validate` против одного и того же `open` и оба запишутся — last-writer-wins, стейт-машина обойдена (можно из `resolved` уехать в `acknowledged`).

**Фикс.** Сделать UPDATE условным по ожидаемому старому статусу и трактовать `RowsAffected()==0` как конфликт `409`:

```sql
UPDATE incident.incidents SET status=$new, ...
WHERE id=$id AND tenant_id=$t AND status=$expectedOld
```
либо выполнить read-validate-write в одной транзакции с `SELECT ... FOR UPDATE`. Тот же приём (CAS) решает и D1 — это общий паттерн для всех переходов состояний в проекте.

---

### D4 — Пул соединений не сконфигурирован — **minor**

`pkg/db.NewPool` парсит DSN, создаёт пул и пингует — но не задаёт ни одного параметра пула:

```go
cfg, err := pgxpool.ParseConfig(dsn)        // db.go:13
pool, err := pgxpool.NewWithConfig(ctx, cfg)
```

Не выставлены `cfg.MaxConns`, `cfg.MinConns`, `cfg.MaxConnLifetime`, `cfg.MaxConnIdleTime`, `cfg.HealthCheckPeriod`. По умолчанию `pgxpool` берёт `MaxConns = max(4, GOMAXPROCS)` и не ограничивает время жизни соединения. Скил `golang-database` (п.11) требует явной конфигурации пула. Последствия дефолтов:
- пять сервисов × `GOMAXPROCS` соединений могут суммарно упереться в лимит Postgres `max_connections` под нагрузкой непредсказуемо;
- без `MaxConnLifetime` долгоживущие соединения не рециклируются (проблема за PgBouncer/при failover'е реплики).

Сейчас всё держится на том, что параметры *можно* передать в DSN (`pool_max_conns=...`), но это нигде не задано и не задокументировано.

**Фикс.** Выставить разумные дефолты в коде с возможностью override из конфига:

```go
cfg.MaxConns = 10
cfg.MinConns = 2
cfg.MaxConnLifetime = 30 * time.Minute
cfg.MaxConnIdleTime = 5 * time.Minute
```
Значения подобрать под `max_connections` Postgres и число реплик каждого сервиса (формула — в `references/performance.md` скила).

---

### D5 — Курсорная пагинация по неуникальному ключу — **minor**

Курсор инцидентов строится по `created_at`:

```go
conds = append(conds, fmt.Sprintf(
    "i.created_at < (SELECT created_at FROM incident.incidents WHERE id = $%d)", idx))
// ... ORDER BY i.created_at DESC
```

([store.go:93-96](services/incident/internal/store/store.go#L93-L96)). Два дефекта:
1. **Неуникальный ключ.** `created_at` не уникален; при нескольких инцидентах с одинаковым таймстампом (массовая генерация из конвейера — вполне реально) строки на границе страницы будут пропущены или продублированы, т.к. `<` строгое и сортировка только по `created_at`.
2. **Несуществующий cursor.** Если переданный `id` не найден, подзапрос вернёт `NULL`, условие `created_at < NULL` — ложно для всех строк → клиент молча получает пустую страницу вместо ошибки/первой страницы.

**Фикс.** Перейти на составной курсор `(created_at, id)` и keyset-пагинацию: `WHERE (i.created_at, i.id) < ($cursorTime, $cursorID) ORDER BY i.created_at DESC, i.id DESC`, передавая в курсоре обе компоненты. Это устраняет и пропуски, и зависимость от существования строки.

---

## Что сделано хорошо (для контекста)

- **Параметризация сплошная — SQL-инъекций нет.** Даже динамический конструктор фильтров в `ListIncidents` использует только плейсхолдеры `$N`; `fmt.Sprintf` подставляет *номера* плейсхолдеров, а значения уходят в `args` ([store.go:74-100](services/incident/internal/store/store.go#L74-L100)). `ORDER BY` нигде не берётся из пользовательского ввода.
- **`rows.Close()` + `rows.Err()`** присутствуют после каждого `Query` во всех трёх крупных store'ах — утечек соединений из-за незакрытых `Rows` нет.
- **NULL обрабатывается осознанно:** incident использует указатели (`*time.Time`, `*string`) для nullable-колонок ([domain/incident.go](services/incident/internal/domain/incident.go)), escalation — `COALESCE(...,'')` в SELECT ([store.go:202](services/escalation/internal/store/store.go#L202)). Оба подхода корректны.
- **`sql.ErrNoRows`/`pgx.ErrNoRows` транслируется в доменную `ErrNotFound`** через `errors.Is` ([incident/store.go:scanIncident](services/incident/internal/store/store.go)) — «не найдено» отделено от технических ошибок.
- **Контекст пробрасывается везде:** все вызовы — `Query(ctx,...)`, `Exec(ctx,...)`, `QueryRow(ctx,...)`. Нарушений из `golang-context` (создание `Background` в середине цепочки) в слое БД нет.
- **Миграции через `golang-migrate`** с отдельной tracking-таблицей на сервис ([pkg/migrate/migrate.go](pkg/migrate/migrate.go)) — внешний инструмент, как и предписывает скил; схемы не генерируются кодом.
- `pool.Ping` при старте — fail-fast на недоступной БД ([db.go:20](pkg/db/db.go#L20)).

---

## Рекомендованный порядок исправлений

1. **D1 + D3** — общий приём (guarded UPDATE / CAS по ожидаемому состоянию): закрывает двойную эскалацию и гонку смены статуса одним паттерном. Наивысший приоритет — это переходы состояний, где гонки уже возможны.
2. **D2** — транзакционный хелпер `withTx` в каждом store; в первую очередь обернуть incident-консьюмер (пересекается с C3 из области 2 — даёт корректный requeue).
3. **D4** — конфигурация пула в `pkg/db` (быстро, общесистемно).
4. **D5** — keyset-пагинация по `(created_at, id)`.

> Кросс-ссылки: D1/D3 — это БД-проявление общей нехватки оптимистичной конкуренции на переходах состояний; D2 напрямую усиливает C3 (область 2) — без транзакции отмена контекста на shutdown рвёт запись на полпути. Конфигурация пула (D4) свяжется с областью наблюдаемости (метрики пула `pgxpool.Stat`).
