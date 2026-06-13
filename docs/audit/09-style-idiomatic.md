# Аудит SREOnCall — Область 9: Стиль / идиоматичность

Дата: 2026-06-13
Область: нейминг, code-style, документация, модернизация под Go 1.26.
Применённые скилы: `golang-naming`, `golang-code-style`, `golang-documentation`, `golang-modernize`.

**Вывод вперёд: по стилю и современности код в очень хорошей форме.** Он уже написан под актуальный Go 1.26 (range-over-int, `any`, `crypto/rand`, `slog`, никаких `ioutil`/`sort.Slice`/`math/rand`/устаревшей крипты), контрол-флоу идиоматичный (early-return, низкая вложенность, именованные поля в композитных литералах), нейминг акронимов и ресиверов корректный. Все находки этой области — **minor**: систематически отсутствуют package-doc-комментарии, есть повсеместный `pkg.Pkg`-стуттер и пара раздутых конструкторов. Критичных/мажорных проблем стиля нет.

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| N1 | **minor** | Package-doc-комментарий (`// Package ...`) отсутствует во **всех 53 пакетах** — задокументированный MUST в `golang-documentation` | все пакеты `pkg/`, `services/*/internal/*` |
| N2 | **minor** | Повсеместный стуттер `pkg.Pkg`: `publisher.Publisher`, `consumer.Consumer`, `handler.Handler`, `store.Store`, `config.Config`, `escalator.Escalator`, `notifier.Notifier`, `monitor.Monitor` | напр. [publisher.go:12](services/ingestion/internal/publisher/publisher.go#L12), [store.go:35](services/scheduling/internal/store/store.go#L35) |
| N3 | **minor** | Конструкторы с >4 параметрами (нет options-struct); крупные файлы хендлеров | [notifier.go:New (8 арг)](services/notification/internal/notifier/notifier.go), [scheduling/handler.go (773 строки)](services/scheduling/internal/handler/handler.go) |
| N4 | **minor** | Enum'ы без zero-value `Unknown`; `mapSeverity` молча превращает незнакомую severity в `Info`; sentinel-строки без префикса пакета; дрейф имён одного концепта между пакетами | [normalize.go:34-44](services/ingestion/internal/handler/normalize.go#L34-L44), [incident/domain/incident.go:13](services/incident/internal/domain/incident.go#L13) |
| N5 | **trivial** | Один `[]interface{}` вместо `[]any`; нет LICENSE/CONTRIBUTING/CHANGELOG | [auth.go:145](pkg/auth/auth.go#L145) |

---

## Детализация

### N1 — Package-doc-комментарии отсутствуют во всех пакетах — **minor**

Проверка `// Package <name> ...` дала **53 из 53 пакетов без package-комментария** — ни `pkg/amqp`, `pkg/auth`, `pkg/domain`, ни один `internal/*` сервиса. По чеклисту `golang-documentation` package-комментарий помечен как **MUST** для любого проекта (библиотека и приложение). Это влияет на навигацию (`go doc`, pkg.go.dev/внутренний godoc показывают пустое описание пакета) и на онбординг: назначение пакета не зафиксировано нигде, кроме имени директории.

Операционного риска нет (поэтому minor), но это 100%-ный и дешёвый в устранении пробел. На фоне того, что **doc-комментарии на экспортируемых символах в основном есть** (в `pkg/` почти все типы и функции описаны — `NewPool`, `Connection`, `Wrap`/`Unwrap`, `Claims`, `Middleware` и т.д.), отсутствие именно package-уровня выглядит как единственный системный недочёт документации кода.

**Фикс.** Добавить по одному package-комментарию на пакет (одна строка-назначение перед `package`), например:

```go
// Package amqp provides the RabbitMQ transport: connection management with
// reconnection, the versioned message Envelope, and topology declaration.
package amqp
```

Для пакетов с богатым контекстом (`auth`, `escalator`, `notifier`) — 2–3 строки. Линтер `revive` (правило `package-comments`) подсветит отсутствие автоматически — закрепить в CI (ср. область 8/T1).

---

### N2 — Повсеместный стуттер `pkg.Pkg` — **minor**

Основные типы названы так же, как их пакет, из-за чего на стороне вызова возникает повтор: `publisher.Publisher`, `consumer.Consumer`, `handler.Handler`, `store.Store`, `config.Config`, `escalator.Escalator`, `notifier.Notifier`, `monitor.Monitor`, `dedup.Deduplicator`. По `golang-naming` («Avoid Stuttering») имя не должно повторять имя пакета — call-site `publisher.Publisher{}` заставляет читать «Publisher» дважды.

Это honest-minor: паттерн `pkg.Pkg` крайне распространён в сервисном Go и многими командами принимается осознанно (особенно для `internal`-пакетов с единственным главным типом). Поэтому — не дефект, а отклонение от конвенции, которое стоит чинить точечно и без фанатизма.

**Фикс (по желанию).** Там, где пакет экспортирует единственный главный тип, тип логично не называть. Канонический ход — оставить тип, но он почти всегда лишний: например, в пакете `publisher` достаточно конструктора `New()` и неэкспортируемого типа, либо тип переименовать по роли. Если решено оставить как есть — добавить короткий комментарий-обоснование (skill это допускает) и зафиксировать в `.modernize`/конвенциях, чтобы ревью не поднимало вопрос повторно. Публичные пакеты `pkg/` стуттером не страдают (`amqp.Publisher`/`Connection`, `auth.Claims`, `metrics.Handler`) — это правильный ориентир.

---

### N3 — Раздутые конструкторы и крупные файлы — **minor**

`golang-code-style` рекомендует ≤4 параметров, дальше — options-struct. Нарушает это `notifier.New`:

```go
notif := notifier.New(st, cache, rl, emailDisp, mmDisp, cfg.SMTPFrom, cfg.FrontendBaseURL, logger) // 8 аргументов
```

([notification/main.go:71](services/notification/cmd/server/main.go#L71)). Восемь позиционных аргументов легко перепутать местами (два соседних — `cfg.SMTPFrom string` и `cfg.FrontendBaseURL string` — одного типа, компилятор подмену не заметит). Остальные конструкторы в пределах нормы (`keycloak.New` — 4, `handler.New` — 3–4).

Дополнительно по организации кода: `scheduling/internal/handler/handler.go` — **773 строки**, `incident/.../handler.go` — 460. Это не баг, но «одна ответственность на файл» нарушается — такой хендлер удобнее разбить по ресурсам (tenants / schedules / overrides / notification-config), тем более что часть из них уже логически сгруппирована роутером.

**Фикс.** Для `notifier.New` — сгруппировать зависимости в `Deps`-struct или применить functional options (`golang-design-patterns`). Это пересекается с областью 1 (ручная конструкторная инъекция, F-кластер): options-struct заодно упростит wiring в `main`. Крупные хендлеры — разнести по файлам внутри пакета (`handler_tenants.go`, `handler_schedules.go` и т.д.) без смены пакета.

---

### N4 — Мелочи нейминга enum'ов и ошибок — **minor**

Несколько отклонений от `golang-naming`, сгруппированы:

1. **Нет zero-value `Unknown`/`Invalid`.** `domain.AlertSeverity`, `domain.AlertStatus`, `incident.Status` — строковые enum'ы без явного нулевого сентинела. Для строковых enum'ов zero — это `""`, что мягче iota-случая, но всё же `var s Status` молча даёт пустую строку.
2. **`mapSeverity` молча понижает неизвестное до `Info`** ([normalize.go:34-44](services/ingestion/internal/handler/normalize.go#L34-L44)): `default: return SeverityInfo`. Незнакомый/опечатанный уровень критичности алерта станет самым низким — это не только нейминг, но и мягкий корректностный смелл (потенциальное занижение приоритета инцидента). Лучше иметь `SeverityUnknown` и логировать неожиданное значение, а не прятать его в `Info`.
3. **Sentinel-строки без префикса пакета:** `errors.New("not found")`, `errors.New("conflict")` — `golang-naming` рекомендует `errors.New("scheduling: not found")`, чтобы происхождение читалось в агрегированном логе. Пересекается с дублированием sentinel'ов (E3, область 4).
4. **Дрейф имён одного концепта:** `incident/domain.AlertFiring`/`AlertResolved` vs `pkg/domain.AlertStatusFiring`/`AlertStatusResolved` — один и тот же статус назван по-разному в двух пакетах (плюс сам дубликат типа, F8 области 1).

**Фикс.** Ввести `SeverityUnknown`/`StatusUnknown` на нулевой позиции и логировать неожиданные значения вместо тихого фолбэка; добавить префикс пакета в sentinel-строки (заодно при выносе общих sentinel'ов из E3); согласовать имена статусов между пакетами при дедупликации типа (F8).

---

### N5 — Тривиальное — **trivial**

- Единственный `[]interface{}` в type-switch разбора claims ([auth.go:145](pkg/auth/auth.go#L145)) → заменить на `[]any` для единообразия (везде в коде уже `any`). Линтер `modernize` это поймает.
- Из документации уровня проекта есть `README.md`, но нет `LICENSE`, `CONTRIBUTING.md`, `CHANGELOG.md`. Для приложения LICENSE — рекомендованный минимум; CONTRIBUTING полезен в связке с CI (область 8) и Makefile (область 1).

---

## Что сделано хорошо (для контекста)

- **Модернизация фактически завершена.** Код уже на идиомах Go 1.26: range-over-int (`for attempt := range 3` — [amqp.go](pkg/amqp/amqp.go)), `any` вместо `interface{}` (единичное исключение N5), `crypto/rand` для токенов, `slog` для логов. Нет ни `math/rand`, ни `rand.Seed`, ни `sort.Slice`, ни `ioutil`, ни устаревших крипто-пакетов. Модернизировать, по сути, нечего — редкая картина.
- **Контрол-флоу идиоматичен:** ранний возврат и guard-клаузы, низкая вложенность, happy-path на минимальном отступе (видно во всех прочитанных хендлерах/сторах).
- **Композитные литералы — с именами полей** (`http.Server{Addr:…, ReadTimeout:…}`), позиционных литералов чувствительных структур нет.
- **Акронимы по конвенции:** `HTTPPort`, `KeycloakJWKSURL`, `DBDSN`, `AMQPURL`, `SchedulingURL` — всё в едином регистре, без `Url`/`Http`/`Json`.
- **Ресиверы консистентны** (`e` у escalator, `s` у store, `h` у handler, `c` у consumer/connection) — один тип, одно имя ресивера во всех методах.
- **Doc-комментарии на экспортируемых символах в основном присутствуют**, особенно в `pkg/` (типы, конструкторы, ключевые методы описаны по делу — _почему/когда_, а не пересказ сигнатуры). Пробел только на package-уровне (N1).
- **Публичные пакеты `pkg/` не стуттерят** (`amqp.Connection`, `auth.Claims`, `metrics.Middleware`) — правильный ориентир для N2.

---

## Рекомендованный порядок исправлений

1. **N1** — добавить package-комментарии (53 пакета, механически; `revive` в CI закрепит). Самый конкретный и полный пробел области.
2. **N4** — `SeverityUnknown` + логирование неожиданных значений (единственный пункт с лёгким корректностным оттенком), префиксы sentinel'ов при выносе из E3.
3. **N3** — `Deps`-struct/options для `notifier.New`; разнести крупные хендлеры по файлам.
4. **N2 + N5** — стуттер чинить точечно/по соглашению; `[]any`, LICENSE/CONTRIBUTING — попутно.

> Кросс-ссылки: N1/N2 закрываются линтером (`revive`) при появлении CI (T1, область 8); N3 пересекается с F-кластером DI (область 1); N4 — с дублированием sentinel'ов E3 (область 4) и типа `AlertStatus` F8 (область 1); все стиль-правила автоматизируются через `golangci-lint` (детали — область инструментов/CI).
