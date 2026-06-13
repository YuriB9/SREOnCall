# Аудит SREOnCall — Область 10: Производительность

Дата: 2026-06-13
Область: узкие места, аллокации в hot-path.
Применённые скилы: `golang-performance`, `golang-benchmark`.

**Дисциплинарная оговорка вперёд (ключевой принцип скилов).** В проекте **нет ни одного бенчмарка** (`grep "func Benchmark"` → 0) и не включён pprof-эндпоинт — значит, ничего не измерено. Интуиция о бутылочных горлышках ошибается в ~80% случаев, поэтому всё ниже — **обоснованные гипотезы, требующие подтверждения профилированием**, а не измеренные факты. При этом скил требует «сначала исключить внешние узкие места»: в этой системе **каждый горячий путь доминируется I/O** (Postgres, RabbitMQ, Redis, S2S-HTTP), поэтому рычаги — про число сетевых round-trip'ов, а не про микро-аллокации. Микрооптимизации CPU/памяти здесь дали бы пренебрежимый эффект на фоне одного INSERT'а — это явно отмечено в разделе «Не оптимизировать».

---

## Приоритизированная сводка

| # | Severity | Находка (гипотеза, требует профилирования) | Ключевые ссылки |
|---|----------|---------|-----------------|
| P1 | **major** | Канал AMQP открывается и закрывается на **каждую** публикацию → +2 round-trip'а к брокеру на сообщение поверх самого publish | [pkg/amqp/amqp.go:publish](pkg/amqp/amqp.go#L113) |
| P2 | **major** | Последовательный I/O без батчинга и конкурентности: вебхук с N алертами обрабатывается серийно прямо в HTTP-запросе; консьюмеры строго однопоточны при `Qos(10)` | [ingestion/handler.go:44-51](services/ingestion/internal/handler/handler.go#L44-L51), C8 |
| P3 | **minor** | Построчные INSERT'ы в цикле (`MergeLabels`) — N round-trip'ов на N лейблов | [incident/store.go:MergeLabels](services/incident/internal/store/store.go) |
| P4 | **minor** | Все S2S-HTTP-клиенты на дефолтном `Transport` (`MaxIdleConnsPerHost=2`) → черн соединений под конкуренцией | [schedclient](services/escalation/internal/schedclient/client.go#L33), [incclient](services/escalation/internal/incclient/client.go#L31), [keycloak](services/scheduling/internal/keycloak/client.go) |
| P5 | **minor** | Двойной `json.Marshal` структуры `Alert` на путь ingestion (envelope + JSONB-колонка) | [ingestion/main.go:SaveRawAlert](services/ingestion/cmd/server/main.go#L159), [envelope.go:Wrap](pkg/amqp/envelope.go#L36) |

> Критичных (измеренных деградаций) нет — измерений нет вовсе. P1/P2 помечены major не по профилю, а потому что это **счётные проблемы числа round-trip'ов**, видимые прямо в коде и масштабирующиеся с нагрузкой.

---

## Детализация

### P1 — Открытие/закрытие AMQP-канала на каждую публикацию — **major**

```go
func (p *Publisher) publish(exchange, routingKey string, body []byte) error {
    ch, err := p.conn.Channel()   // amqp.go — channel.open: round-trip к брокеру
    if err != nil { return err }
    defer ch.Close()              // channel.close: ещё один round-trip
    return ch.PublishWithContext(context.Background(), exchange, routingKey, false, false, ...)
}
```

Каждое опубликованное сообщение создаёт **новый** AMQP-канал и тут же его закрывает. `channel.open` и `channel.close` — это синхронные команды протокола, то есть **два дополнительных сетевых round-trip'а к RabbitMQ на каждое сообщение** поверх самой публикации. Каналы AMQP специально сделаны дешёвыми именно для переиспользования; пересоздавать их на каждый publish — антипаттерн, который втрое увеличивает сетевые обращения горячего пути.

Затрагивает все продюсеры: ingestion (на **каждый** алерт), incident (на каждое создание/обновление инцидента), escalation (на каждый trigger/exhaust). На пике алертов это самый дорогой структурный оверхед публикации.

Дополнительно усугубляется **C4** (область 2): `Connection.Channel()` берёт мьютекс и под ним может уйти в reconnect со `time.Sleep` — то есть при сбое брокера создание каналов ещё и сериализуется.

**Фикс.** Держать долгоживущий канал на `Publisher` (один на продюсера), защищённый мьютексом, и переоткрывать только при ошибке/закрытии; либо небольшой пул каналов. Для устойчивости — publisher confirms на переиспользуемом канале. Это убирает 2 round-trip'а с каждого сообщения. Обязательно подтвердить бенчмарком `publish` до/после (`-benchmem`, `benchstat`).

---

### P2 — Последовательный I/O без батчинга и без конкурентности — **major**

Два проявления одной проблемы — пропускная способность ограничена суммой последовательных round-trip'ов.

**(а) Ingestion: вся пачка алертов — синхронно в HTTP-запросе.**

```go
func (h *Handler) HandleAlertmanager(w http.ResponseWriter, r *http.Request) {
    ...
    if err := h.processAlerts(r.Context(), alerts); err != nil { ... }  // блокирует ответ
    w.WriteHeader(http.StatusOK)
}
```

`processAlerts` → цикл `processOne` по каждому алерту, и каждый `processOne` делает **Redis SetNX + Postgres INSERT + AMQP publish последовательно** ([handler.go:44-83](services/ingestion/internal/handler/handler.go#L44-L83)). Webhook Alertmanager штатно несёт десятки алертов в одном теле → отправитель (Alertmanager) блокируется на `N × (Redis + PG + AMQP-с-двумя-RTT по P1)` и может упереться в свой таймаут, ретраить и дублировать нагрузку. INSERT'ы в `raw_alerts` не батчатся, Redis не пайплайнится, публикации не группируются.

**(б) Консьюмеры: `Qos(10)`, но обработка строго по одному** (C8, область 2). Каждое сообщение incident-консьюмера — это ~5–8 последовательных запросов в БД (`GetGroupingRule` → `FindOpenIncidentByGroupKey` → `CreateIncident` → `MergeLabels` → `AppendHistory` → `GetIncident` → `AttachAlert`), и следующее сообщение не начинается, пока не закоммичено текущее. Префетч 10 не даёт параллелизма (см. C8).

**Фикс.**
- ingestion: батчить запись `raw_alerts` (`pgx.Batch` или `COPY`), пайплайнить Redis, группировать публикации на переиспользуемом канале (P1). Опционально — принимать вебхук, складывать в канал и отвечать `202`, обрабатывая асинхронно (но тогда нужна устойчивость очереди — взвесить с гарантиями доставки).
- консьюмеры: bounded worker-pool через `errgroup.SetLimit(n)` с `Qos(n)` (ср. C8) — обрабатывать до n сообщений параллельно, сохраняя backpressure.
- сначала бенчмарк `processOne`/обработки сообщения и нагрузочный тест вебхука с реалистичным N.

---

### P3 — Построчные INSERT'ы в цикле — **minor**

```go
for k, v := range labels {
    s.db.Exec(ctx, `INSERT INTO incident.incident_labels (...) VALUES ($1,$2,$3)
                    ON CONFLICT ... DO UPDATE ...`, incidentID, k, v)  // round-trip на каждый лейбл
}
```

([incident/store.go:MergeLabels](services/incident/internal/store/store.go)). N лейблов = N round-trip'ов в БД, и это умножается на P2(б) (внутри обработки каждого сообщения). Аллокационно дёшево, дорого по сети.

**Фикс.** Один multi-row INSERT (`unnest($1::text[], $2::text[])` или `pgx.Batch`) — один round-trip вместо N. То же проверить в других циклах записи.

---

### P4 — Дефолтный HTTP-Transport на S2S-клиентах — **minor**

Все межсервисные клиенты создаются как `&http.Client{Timeout: 10 * time.Second}` без явного `Transport` ([schedclient:33](services/escalation/internal/schedclient/client.go#L33), [incclient:31](services/escalation/internal/incclient/client.go#L31), [notification/schedclient](services/notification/internal/schedclient/client.go), [keycloak](services/scheduling/internal/keycloak/client.go), [mattermost](services/notification/internal/dispatcher/mattermost.go)). Дефолтный `http.Transport` держит `MaxIdleConnsPerHost = 2` — под конкуренцией это значит, что больше двух одновременных вызовов к scheduling/incident не переиспользуют keep-alive и платят за новый TCP+TLS-handshake.

Чувствительность разная: notification ходит в scheduling через `tenantcache` (TTL 5 мин) — смягчено; **escalation вызывает `GetOnCall` в scheduling на каждый trigger без кэша** — здесь черн соединений заметнее.

**Фикс.** Общий настроенный `http.Transport` (`MaxIdleConnsPerHost` под ожидаемую конкуренцию, `MaxIdleConns`, `IdleConnTimeout`), разделяемый клиентами — естественно ложится на общий `pkg/httpclient` из F3 (область 1).

---

### P5 — Двойная сериализация `Alert` на пути ingestion — **minor**

На каждый алерт структура `Alert` маршалится дважды: в `pkgamqp.Wrap` (в payload конверта) и в `SaveRawAlert` (в JSONB-колонку `raw_alerts.payload` — `json.Marshal(alert)`, [ingestion/main.go:160](services/ingestion/cmd/server/main.go#L160)). Плюс анмаршалинг тела вебхука. Это лишние аллокации, но **на фоне Redis+PG+AMQP они пренебрежимы** — выношу как minor и только для полноты.

**Фикс (низкий приоритет).** Маршалить `Alert` один раз и переиспользовать `json.RawMessage` для обоих потребителей. Делать только если профиль ingestion реально покажет аллокации `json` в топе — иначе не трогать.

---

## Не оптимизировать (явно)

Скил требует не тратить усилия там, где их съест I/O. Эти места **аллокационно-аккуратны и пренебрежимы** относительно окружающих round-trip'ов — менять их нет смысла:

- **`computeFingerprint` / `computeGroupKey`** ([normalize.go](services/ingestion/internal/handler/normalize.go), [incident/consumer.go](services/incident/internal/consumer/consumer.go)) уже преаллоцируют срез ключей, используют `strings.Builder` и один `sha256`. Стоимость — десятки наносекунд против миллисекунд PG/Redis рядом. Трогать незачем.
- **`DeliveryMode: Persistent`** — осознанная плата за надёжность доставки, а не баг; не «оптимизировать» в transient.
- Нет `reflect.DeepEqual`, нет логирования в тесных циклах (консьюмер логирует раз на сообщение = на пачку I/O — приемлемо).

---

## Что сделано хорошо (для контекста)

- **Кэширование дорогих обращений есть:** `tenantcache` (TTL 5 мин) избавляет notification от вызова scheduling на каждое уведомление — правильный паттерн «work avoidance» (осталось добавить singleflight против stampede, C7).
- **Дедуп отсекает лишнюю работу ниже по потоку** (Redis SetNX) и при этом инструментирован (`dedupHits`/`dedupMisses`) — единственный измеряемый кусок производительности в проекте.
- **Префетч `Qos`** выставлен (хоть и не задействован, C8) — основа для воркер-пула уже есть.
- **`MaxBytesReader` на вебхуках** ограничивает размер тела (4 МБ) — защита от патологических аллокаций на входе.
- Преаллокация срезов по известной длине в нормализации (`make([]domain.Alert, 0, len(p.Alerts))`, `make(map..., len(p.Tags)+3)`) — аллокационная гигиена соблюдается.

---

## Рекомендованный порядок действий

1. **Сначала — измеримость (без неё остальное вслепую):** добавить бенчмарки на горячие пути (`publish`, `processOne`, обработка сообщения консьюмером, `computeFingerprint`) и включить защищённый pprof-эндпоинт (ср. область 7). Это предпосылка ко всему.
2. **P1** — переиспользуемый канал публикации: высокая уверенность, локальная правка в `pkg/amqp`, измеримый выигрыш на каждом сообщении.
3. **P2** — батчинг записи в ingestion + воркер-пул консьюмеров (вместе с C8): снимает потолок пропускной способности конвейера.
4. **P3 + P4** — multi-row INSERT и настроенный общий `Transport` (последнее — внутри `pkg/httpclient`, F3).
5. **P5** — только если профиль подтвердит.

> Кросс-ссылки: P1 ↔ C4 (мьютекс при создании канала, область 2); P2 ↔ C8 (последовательный консьюмер) и D4 (нетюнингованный пул, область 3 — реальный governor пропускной способности под конкуренцией); P4 ↔ F3 (общий HTTP-клиент, область 1); включение pprof ↔ область 7. Все выводы — гипотезы до профилирования; benchstat-вывод обязателен в коммите каждой оптимизации.
