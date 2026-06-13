# Аудит SREOnCall — Область 6: Надёжность / паники

Дата: 2026-06-13
Область: nil-разыменования, append-алиасинг, конкурентный доступ к map, `defer` в циклах.
Применённый скил: `golang-safety`.

**Вывод вперёд: эта область — самая чистая из проверенных.** Код написан защитно: проверены все классические ловушки `golang-safety` — паттернов с гарантированной паникой или порчей данных практически нет. Найдена одна реальная проблема надёжности (неограниченная кардинальность Prometheus-меток → исчерпание памяти) и одна мелочь (проглоченные ошибки чтения, дающие `null`-ответы). Большая часть отчёта — подтверждение того, что опасные паттерны **отсутствуют**, с указанием, где именно защита сработала.

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| R1 | **major** | `r.URL.Path` используется как метка Prometheus → неограниченная кардинальность (ID в путях) → рост памяти без предела, по сути memory-DoS | [pkg/metrics/metrics.go:38-42](pkg/metrics/metrics.go#L38-L42) |
| R2 | **minor** | Проигнорированные ошибки чтения возвращают nil-указатель → клиент получает `null`-тело с 200/201 вместо ошибки | [escalation/handler.go:254](services/escalation/internal/handler/handler.go#L254), [:317](services/escalation/internal/handler/handler.go#L317) |

Паник-классов (`critical`/`major` по nil-deref, nil-map-write, append-aliasing, гонкам по map, `defer`-в-цикле) **не обнаружено** — детали в разделе «Проверено и безопасно».

---

## Детализация

### R1 — Неограниченная кардинальность Prometheus-меток — **major**

Middleware метрик помечает каждый запрос сырым путём URL:

```go
timer := prometheus.NewTimer(requestDuration.WithLabelValues(service, r.Method, r.URL.Path))
next.ServeHTTP(rw, r)
timer.ObserveDuration()
requestsTotal.WithLabelValues(service, r.Method, r.URL.Path, strconv.Itoa(rw.status)).Inc()
```

([metrics.go:38-42](pkg/metrics/metrics.go#L38-L42)). Маршруты проекта содержат идентификаторы прямо в пути: `/api/incidents/v1/{tenant_id}/incidents/{incidentId}`, `/api/escalations/v1/{tenant}/incidents/{incidentId}/state` и т.д. `r.URL.Path` — это **конкретный** путь с реальными UUID/slug, поэтому каждый новый инцидент/тенант порождает **новую таймсерию** в `http_requests_total` и `http_request_duration_seconds`.

Клиент Prometheus хранит все series в памяти процесса бессрочно — они не вытесняются. Последствия:
- **Утечка памяти при нормальном трафике**: число series растёт пропорционально числу когда-либо запрошенных ID. На активной системе это монотонный рост RSS до OOM.
- **DoS-вектор**: метрика пишется после `ServeHTTP` независимо от статуса, поэтому даже запросы на несуществующие `/api/.../incidents/<random>` (отдающие 404) раздувают кардинальность. Неаутентифицированный... нет — эндпоинты под auth, но любой валидный пользователь может за минуту создать миллионы series, перебирая случайные ID в URL.

Это классический антипаттерн «high-cardinality label» — гарантированная деградация памяти, попадающая в `golang-safety` через ресурсную безопасность.

**Фикс.** Метить запрос **шаблоном маршрута**, а не конкретным путём. chi отдаёт шаблон после матчинга:

```go
pattern := chi.RouteContext(r.Context()).RoutePattern() // "/api/incidents/v1/{tenant_id}/incidents/{incidentId}"
requestsTotal.WithLabelValues(service, r.Method, pattern, strconv.Itoa(rw.status)).Inc()
```

`RoutePattern()` доступен после того, как роутер сматчил запрос — для chi-middleware это значит обернуть наблюдение так, чтобы шаблон читался **после** `next.ServeHTTP` (либо использовать `chiMiddleware`-совместимый порядок). Кардинальность тогда ограничена числом маршрутов (десятки), а не числом ID. На нематченных путях (404 до роутинга) `RoutePattern()` пуст — подставлять литерал `"unmatched"`, чтобы и тут кардинальность была константной.

> Находка пересекается с областью наблюдаемости (это и про качество метрик), но по сути своей — ресурсная утечка, поэтому отнесена сюда.

---

### R2 — Проглоченные ошибки чтения → `null`-ответы — **minor**

После записи хендлеры перечитывают состояние, игнорируя ошибку и не проверяя на nil:

```go
// AttachPolicy
st, _ := h.store.GetEscalationStateByIncident(r.Context(), tenant(r), incidentID)
writeJSON(w, http.StatusCreated, st)   // escalation/handler.go:254
```
(аналогично `ManualEscalate`, [handler.go:317](services/escalation/internal/handler/handler.go#L317)).

Паники здесь нет: `json.Marshal` от nil-указателя даёт `null`, а не крэш — поэтому это не дефект безопасности памяти, а проблема корректности/наблюдаемости. Если `GetEscalationStateByIncident` вернёт ошибку (транзиентный сбой БД) или `ErrNotFound` (гонка удаления), клиент получит `201 Created` / `200 OK` с телом `null` вместо `5xx`/`404`, а ошибка нигде не залогируется (нарушение single-handling rule — ср. E4 области 4).

**Фикс.** Проверять ошибку и nil:

```go
st, err := h.store.GetEscalationStateByIncident(...)
if err != nil {
    h.logger.Error("read escalation state after attach", "incident_id", incidentID, "err", err)
    writeError(w, http.StatusInternalServerError, "internal error")
    return
}
writeJSON(w, http.StatusCreated, st)
```

---

## Проверено и безопасно (почему классических паник нет)

Каждый пункт чеклиста `golang-safety` проверен; ниже — где сработала защита.

- **nil-разыменования возвращаемых указателей — нет.** Опасные места проверены: `triggerTier` использует `result.UserID` только в ветке `err == nil` ([escalator.go](services/escalation/internal/escalator/escalator.go) — `GetOnCall`); incident-консьюмер читает `inc, _ := GetIncident(...)` и далее **явно** проверяет `if inc != nil` ([consumer.go:183](services/incident/internal/consumer/consumer.go#L183), [:225](services/incident/internal/consumer/consumer.go#L225)). Единственные неприкрытые `_`-чтения (R2) дают `null`, а не панику.
- **Запись в nil-map — нет.** Все карты инициализируются `make` до записи: `loadLabels` (`inc.Labels = make(...)` перед циклом, [store.go:237](services/incident/internal/store/store.go#L237)), `NormalizeGrafana` (`labels := make(map[string]string, ...)` перед заполнением из tags, [grafana.go:36](services/ingestion/internal/handler/grafana.go#L36)). Карты из ненадёжного ввода, которые могут прийти `nil` (`amAlert.Labels`, `Alert.Labels`), везде только **читаются** (`labels["severity"]`, `range labels`) — чтение/range nil-map в Go безопасны и дают нули/0 итераций. Через весь конвейер (fingerprint → groupKey → MergeLabels) nil-labels проходят корректно: `MergeLabels` на пустой карте делает ранний `return nil` ([store.go:MergeLabels](services/incident/internal/store/store.go)).
- **append-алиасинг — нет.** Все `append` пишут в собственные накопительные срезы (`result`, `out`, `parts`, `args`, `conds`, `Statuses`, `Tiers`), построенные тут же; общих backing-массивов между «чужими» срезами, мутирующих друг друга, не найдено.
- **Конкурентный доступ к map — нет.** Единственные разделяемые между горутинами карты защищены: `tenantcache.data` под `sync.Mutex` (lock не держится через I/O — см. область 2), векторы Prometheus потокобезопасны внутри клиента. Хендлеры stateless, фоновые горутины (консьюмеры/монитор) обрабатывают по одному сообщению — общих мутируемых карт нет.
- **`defer` в циклах — нет.** Все `defer rows.Close()` / `defer tx.Rollback` / `defer ch.Close()` стоят на входе функции, по одному на вызов; ни одного `defer` внутри тела `for`. Построчные операции в циклах (`MergeLabels`) ресурсов не открывают.
- **Type-assertion'ы — все comma-ok.** Голых `x.(T)` нет (`grep` пуст); разбор JWT-клеймов (`mapStr`, `mapStrSlice`) и чтения из контекста (`FromContext`, `MethodFromContext`, `TenantFromContext`) используют форму `v, ok :=`.
- **Границы срезов** в курсорной пагинации защищены условием `len(result) > f.PageSize` перед `result[:f.PageSize]` и `result[f.PageSize-1]` ([store.go:141-144](services/incident/internal/store/store.go#L141-L144)).

---

## Связанные риски надёжности из других областей (не дублируются здесь)

- **Паника в фоновой горутине роняет процесс** — консьюмеры/монитор без `recover` (E2, область 4), без supervisor-рестарта (C1, область 2). Это главный «панический» риск проекта, но его корень — отсутствие recovery-барьера, а не опасный паттерн в самом коде обработки; поэтому он разобран там.
- **HTTP-хендлеры без `Recoverer`** в 3 сервисах (E1, область 4) — паника в хендлере не превращается в `500`.
- **Отмена контекста рвёт нетранзакционную запись** (C3/D2) — это про целостность данных при shutdown, разобрано в областях 2 и 3.

---

## Рекомендованный порядок исправлений

1. **R1** — заменить `r.URL.Path` на `chi.RouteContext(...).RoutePattern()` в `pkg/metrics`: устраняет утечку памяти и DoS-вектор; правка локальная в одном файле, эффект на все 5 сервисов сразу.
2. **R2** — проверять ошибку/nil в post-write чтениях escalation-хендлера (заодно закрывает кусок E4).
