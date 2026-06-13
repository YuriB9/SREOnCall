# Аудит SREOnCall — Область 4: Обработка ошибок

Дата: 2026-06-13
Область: обёртки `%w`, `errors.Is`/`errors.As`, sentinel-ошибки, структурное логирование `slog`.
Применённый скил: `golang-error-handling`.

Базовая дисциплина ошибок в проекте **выдержана**: соблюдён single-handling rule (store'ы оборачивают и возвращают, хендлеры логируют и отвечают — двойного логирования нет), `%w` используется в оборачивающих `Errorf`, sentinel'ы матчатся через `errors.Is` (58 вхождений `errors.Is`/`errors.As`), везде `slog` без legacy-логгеров и `fmt.Println`. Основные проблемы — не в стиле, а в **устойчивости**: три сервиса без panic-recovery, фоновые горутины без `recover`, и потеря семантики ошибок на межсервисных границах.

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| E1 | **major** | 3 из 5 сервисов не подключают panic-recovery middleware → паника в HTTP-хендлере роняет весь процесс | [escalation/main.go:113](services/escalation/cmd/server/main.go#L113), [notification/main.go:127](services/notification/cmd/server/main.go#L127), [scheduling/main.go:116](services/scheduling/cmd/server/main.go#L116) |
| E2 | **major** | Фоновые горутины (консьюмеры, монитор) не имеют `recover` → паника на одном сообщении/тике убивает сервис, и (см. C1) он не перезапускается | [incident/consumer.go:96](services/incident/internal/consumer/consumer.go#L96), [monitor.go:43](services/escalation/internal/monitor/monitor.go#L43) |
| E3 | **major** | HTTP-клиенты не маппят 404 в sentinel + `ErrNotFound` продублирован в 4 пакетах → межсервисно «не найдено» неотличимо от технической ошибки, `errors.Is` через границу невозможен | [escalation/incclient.go:48-50](services/escalation/internal/incclient/client.go#L48-L50), [escalation/schedclient.go:48-50](services/escalation/internal/schedclient/client.go#L48-L50) |
| E4 | **minor** | Проглоченные ошибки `AppendHistory` (`_ =`) в 8 местах — записи аудит-трейла теряются молча (ни лог, ни возврат) | [escalator.go:111](services/escalation/internal/escalator/escalator.go#L111), [incident/consumer.go:161](services/incident/internal/consumer/consumer.go#L161), [incident/handler.go:186](services/incident/internal/handler/handler.go#L186) |
| E5 | **minor** | Несогласованный ключ ошибки в `slog`: `"err"` (99) vs `"error"` (15) → фрагментация группировки в агрегаторе логов | по всему коду; пример [scheduling/main.go:108](services/scheduling/cmd/server/main.go#L108) |
| E6 | **minor** | `http.Error(w, err.Error(), …)` отдаёт внутренний текст ошибки клиенту | [incident/handler.go:174](services/incident/internal/handler/handler.go#L174) |

---

## Детализация

### E1 — Три сервиса без panic-recovery middleware — **major**

Только ingestion и incident подключают защиту от паники:

```go
r.Use(chiMiddleware.Recoverer)   // ingestion/main.go:90, incident/main.go:89
```

В роутерах escalation, notification и scheduling из middleware стоит **только** `pkgmetrics.Middleware(...)` — `Recoverer` отсутствует ([escalation/main.go:113](services/escalation/cmd/server/main.go#L113), [notification/main.go:127](services/notification/cmd/server/main.go#L127), [scheduling/main.go:116](services/scheduling/cmd/server/main.go#L116)). Последствие: любой `nil`-разыменование или out-of-range в хендлере этих сервисов не превращается в `500`, а паникует через всю цепочку middleware. `net/http` ловит панику отдельной горутины-соединения и не роняет процесс — **но** при этом `pkgmetrics.Middleware` не успевает записать метрику (паника проходит сквозь `defer`-цепочку нештатно), а главное — расходится поведение между сервисами: половина парка отдаёт `500`, половина рвёт соединение без ответа и без метрики. Скил `golang-error-handling` (panic/recover на границе) требует единого recovery-барьера на входе.

**Фикс.** Добавить `r.Use(chiMiddleware.Recoverer)` в три недостающих сервиса (а ещё лучше — вынести единый набор middleware в общий хелпер `pkg/httpserver`, ср. F4 из области 1, чтобы recovery нельзя было «забыть» при заведении нового сервиса).

---

### E2 — Фоновые горутины без `recover` — **major**

Консьюмеры и монитор обрабатывают сообщения в цикле без какого-либо барьера паники:

```go
func (c *Consumer) handle(ctx context.Context, msg amqp091.Delivery) {
    var alert domain.Alert
    env, err := pkgamqp.Unwrap(msg.Body, &alert)   // incident/consumer.go:96
    ...
    processErr = c.handleFiring(ctx, alert, env.TenantID)  // далее — карты labels, разыменования
}
```

`handleFiring` работает с `alert.Labels` (map), `computeGroupKey`, указателями — любой неожиданный `nil`/паника в обработке **одного** сообщения не нэкается, а раскручивается до верха горутины. Поскольку у горутины нет `recover` (и нет supervisor-петли — C1 из области 2), паника убивает весь процесс. То есть единственное «отравленное» сообщение, вызвавшее панику, останавливает сервис целиком; после рестарта оно переотдаётся (Nack/requeue в области шины) и роняет его снова — потенциальный crash-loop. То же относится к `monitor.step` ([monitor.go:43](services/escalation/internal/monitor/monitor.go#L43)) — паника на одном просроченном state валит весь монитор.

`net/http.Recoverer` тут не помогает — это другой стек, не HTTP.

**Фикс.** Обернуть обработку одной единицы работы в recover, конвертирующий панику в ошибку (→ Nack без requeue / в DLQ), чтобы изолировать «отравленное» сообщение:

```go
func (c *Consumer) handle(ctx context.Context, msg amqp091.Delivery) {
    defer func() {
        if r := recover(); r != nil {
            c.logger.Error("panic in handler", "panic", r, "stack", string(debug.Stack()))
            _ = msg.Nack(false, false) // не requeue — иначе crash-loop
        }
    }()
    ...
}
```

Связка с C1/C2 (область 2): recover на уровне единицы работы + supervisor-петля на уровне `Run` = горутина не умирает ни от паники в сообщении, ни от обрыва соединения.

---

### E3 — Потеря семантики ошибок на межсервисных границах — **major**

Внутри сервиса работа с sentinel'ами образцовая: `escalator` всюду делает `errors.Is(err, store.ErrNotFound)` и пробрасывает sentinel дальше ([escalator.go:64-65](services/escalation/internal/escalator/escalator.go#L64-L65) и др.). Но на границе HTTP-клиентов семантика теряется:

```go
if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("incclient: status %d for incident %s", resp.StatusCode, incidentID)
}   // incclient/client.go:48-50 — 404 неотличим от 500
```

`incclient` и escalation-`schedclient` сворачивают **любой** не-200 в один безликий `fmt.Errorf` — вызывающий код не может `errors.Is(err, ErrNotFound)` и отличить «инцидента нет» от «scheduling упал». Показательно, что notification-`schedclient` это уже делает правильно — мапит 404 в `return nil, nil` ([notification/schedclient.go:54-56](services/notification/internal/schedclient/client.go#L54-L56)), то есть в проекте есть и правильный, и неправильный паттерн одновременно.

Усугубляет проблему дублирование sentinel'а: `ErrNotFound` объявлен независимо в [incident/store.go:17](services/incident/internal/store/store.go#L17), [escalation/store.go:13](services/escalation/internal/store/store.go#L13), [scheduling/store.go:15](services/scheduling/internal/store/store.go#L15) и [notification/store.go:12](services/notification/internal/store/store.go#L12). Это **четыре разных значения** — `errors.Is` между сервисами не сработает в принципе, даже если клиент захочет вернуть «свой» `ErrNotFound`.

**Фикс.**
1. Вынести общие sentinel'ы (`ErrNotFound`, `ErrConflict`) в общий пакет (`pkg/errs` или рядом с `pkg/domain`), чтобы значение было одно на монорепо (ср. F1/F6 области 1).
2. В HTTP-клиентах мапить статусы в эти sentinel'ы: `404 → ErrNotFound`, `409 → ErrConflict`, прочее — обёрнутая ошибка с `%w`. Тогда `errors.Is(err, errs.ErrNotFound)` заработает сквозь сетевую границу, и вызывающий код (например, `escalator` при ручном attach) сможет корректно реагировать на «инцидент удалён».

---

### E4 — Проглоченные ошибки записи в аудит-историю — **minor**

Запись в журнал событий повсеместно отбрасывается через `_ =`:

```go
_ = e.store.AppendHistory(ctx, &domain.EscalationHistory{...})   // escalator.go:111, 161, 219
_ = c.store.AppendHistory(ctx, &incdomain.HistoryEntry{...})     // incident/consumer.go:161, 217
_ = h.store.AppendHistory(r.Context(), &incdomain.HistoryEntry{...}) // incident/handler.go:186, 291, 347
```

Ошибка не логируется и не возвращается — нарушение п.1 скила («returned errors MUST always be checked»). История инцидента/эскалации — это аудит-трейл (кто и когда подтвердил/эскалировал); при сбое записи он молча теряет события, и расследование инцидента постфактум покажет «дыры» без следа в логах.

Намерение «история не должна ломать основную операцию» — разумно (её незачем возвращать как фатальную ошибку), но это **не значит «проглотить»**. По single-handling rule её нужно **залогировать** на уровне Warn:

```go
if err := e.store.AppendHistory(ctx, &domain.EscalationHistory{...}); err != nil {
    e.logger.Warn("append history failed", "incident_id", st.IncidentID, "err", err)
}
```

(`escalator` уже логирует так провалы публикации — здесь та же планка.)

---

### E5 — Несогласованный ключ ошибки в slog — **minor**

В структурных логах ошибка кладётся под двумя разными ключами: `"err"` — 99 вхождений, `"error"` — 15. Пример `"error"`-варианта — JSON-ответы и часть логов; основной код использует `"err"`. Для агрегатора логов (Loki/ELK) это **два разных поля**: фильтр `err=...` не увидит записи с `error=...`, группировка и алерты по ошибкам фрагментируются. Скил (п.15, low-cardinality, стабильные шаблоны) предполагает единый ключ.

**Фикс.** Выбрать один ключ (идиоматично — `"err"`, уже доминирует) и привести 15 мест к нему. Дёшево закрепить `forbidigo`/grep-правилом в CI, либо единым хелпером логирования в `pkg/logger`.

---

### E6 — Внутренний текст ошибки уходит клиенту — **minor**

```go
http.Error(w, err.Error(), http.StatusUnprocessableEntity)   // incident/handler.go:174
```

Здесь `err` — это ошибка `statemachine.Validate` (доменное сообщение вида «invalid transition…»), поэтому риск утечки технических деталей низкий. Но паттерн «отдать `err.Error()` в HTTP-ответ» нарушает п.14 («never expose technical errors to users»): при рефакторинге сюда легко может прийти обёрнутая ошибка с внутренними подробностями (имена таблиц, DSN-фрагменты). Это единственное такое место — остальные хендлеры отдают статичные строки (`"invalid body"`, `"internal error"`), что правильно.

**Фикс.** Отдавать клиенту стабильное доменное сообщение (например, из типизированной ошибки стейт-машины), технические детали — только в лог.

---

## Что сделано хорошо (для контекста)

- **Single-handling rule соблюдён.** Store'ы не логируют — они оборачивают и возвращают (`grep` по `store/*.go` на logger/slog пуст); хендлеры и консьюмеры логируют один раз на границе и отвечают/нэкают. Дублирующихся логов одной ошибки нет.
- **`errors.Is` используется широко и правильно** (58 вхождений `errors.Is`/`errors.As`), особенно в `escalator` для `store.ErrNotFound` — внутрисервисная семантика ошибок чистая.
- **`%w` в оборачивающих `Errorf`.** Случаи без `%w` — это создание leaf-ошибок (валидация длительности в `rotation`, HTTP-статусы в клиентах, «channel closed» в консьюмерах), где оборачивать нечего — корректное применение, а не потеря цепочки.
- **Только `slog`.** Ни `log.Printf`, ни `fmt.Println` в проде; логгер инжектится конструкторами (кроме scheduling — см. F5 области 1).
- **Сообщения ошибок — lowercase, без пунктуации**, с префиксом пакета (`schedclient:`, `consumer:`, `advance:`) — удобно для трассировки происхождения.
- **Сервис-локальные sentinel'ы** (`ErrNotFound`, `ErrConflict`, `ErrEmptyRotation`) объявлены как преаллоцированные `var` — идиоматично; проблема только в их дублировании и непрокидывании через клиентов (E3).

---

## Рекомендованный порядок исправлений

1. **E1 + E2** — закрыть recovery-барьеры (HTTP middleware в 3 сервисах + `recover` в фоновых горутинах): защита от падения процесса, особенно в связке с C1 (нет авто-рестарта).
2. **E3** — общие sentinel'ы в `pkg/` + маппинг статусов в HTTP-клиентах: восстанавливает `errors.Is` через сетевую границу.
3. **E4** — заменить `_ = AppendHistory` на логирование Warn: сохраняет видимость потерь аудита.
4. **E5 + E6** — единый ключ `"err"` и отказ от `err.Error()` в HTTP-ответах: косметика, но важна для логов и безопасности.

> Кросс-ссылки: E1/E2 усиливают C1/C2 (область 2 — без recover и без supervisor горутина не переживает ни панику, ни обрыв); E3 опирается на общий пакет ошибок из F1/F6 (область 1). Отсутствие структурного request-logging middleware (метод/путь/статус/длительность) — отнесено в область наблюдаемости.
