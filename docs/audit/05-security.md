# Аудит SREOnCall — Область 5: Безопасность

Дата: 2026-06-13
Область: auth/JWT/JWKS, SQL-инъекции, секреты, валидация входных данных вебхуков.
Применённые скилы: `golang-security` (ultrathink), `golang-safety`.

Каждая находка прослежена по потоку данных от точки входа (trust boundary) до чувствительной операции, severity скорректирована с учётом вышестоящих защит. Severity дана в основной шкале аудита (critical/major/minor) с пометкой уровня по `golang-security` (DREAD-классы) в скобках.

Общая картина: **гигиена базовых классов уязвимостей хорошая** — SQL-инъекций нет (параметризация, см. область 3), токены генерируются `crypto/rand`, тело вебхука ограничено, секреты не хардкодятся и не утекают в логи, type-assertion'ы безопасны. Реальные проблемы — в **аутентификации/авторизации и SSRF**: fail-open при пустом JWKS, неполная валидация JWT, god-key с небезопасным сравнением и SSRF через подконтрольный тенанту URL вебхука.

---

## Приоритизированная сводка

| # | Severity | Находка | Ключевые ссылки |
|---|----------|---------|-----------------|
| S1 | **major** (High — fail-open) | При пустом `KEYCLOAK_JWKS_URL` middleware заменяется на passthrough — сервис работает вообще без аутентификации на всех защищённых маршрутах | [incident/main.go:71-84](services/incident/cmd/server/main.go#L71-L84); все 5 `main.go` |
| S2 | **major** (Medium — SSRF) | `MattermostWebhookURL` задаётся тенант-админом без allowlist/блокировки приватных адресов; notification-сервис POST'ит на произвольный URL → SSRF во внутреннюю сеть | [scheduling/handler.go:705-707](services/scheduling/internal/handler/handler.go#L705-L707), [notifier.go:317-326](services/notification/internal/notifier/notifier.go#L317-L326) |
| S3 | **major** (Medium — timing/god-key) | Admin-key сравнивается через `==` (не constant-time) и даёт неограниченный кросс-тенант обход авторизации | [pkg/auth/auth.go:102](pkg/auth/auth.go#L102) |
| S4 | **major** (Medium — broken auth) | JWT валидируется без проверки `aud`/`iss`, без allowlist алгоритмов и без требования `exp` → токен другого клиента realm / без срока годности проходит | [pkg/auth/auth.go:114](pkg/auth/auth.go#L114) |
| S5 | **minor** (Low — MITM) | Схема `KEYCLOAK_JWKS_URL` не форсится на https → ключи JWKS могут тянуться по http и быть подменены MITM | [pkg/auth/auth.go:90](pkg/auth/auth.go#L90) |
| S6 | **minor** (Low — DoS) | Нет rate-limiting на входе (вебхуки/auth) — поверхность для флуда/брутфорса токенов | [ingestion/main.go](services/ingestion/cmd/server/main.go) |

---

## Детализация

### S1 — Fail-open аутентификация при пустом JWKS URL — **major** (High)

Во всех пяти сервисах middleware подключается по условию, а ветка «иначе» — это **проходной** обработчик:

```go
if cfg.KeycloakJWKSURL != "" {
    mw, err := pkgauth.Middleware(cfg.KeycloakJWKSURL, cfg.AdminKey)
    ...
    authMW = mw
} else {
    authMW = func(next http.Handler) http.Handler { return next }  // ← НЕТ аутентификации
}
```

([incident/main.go:71-84](services/incident/cmd/server/main.go#L71-L84), аналогично в остальных). `authMW` навешивается на всю группу `/api/...`. Если `KEYCLOAK_JWKS_URL` не задан (опечатка в манифесте, незаполненный secret, ошибка раскатки), сервис поднимается штатно и обслуживает **все защищённые эндпоинты без какой-либо проверки личности** — это fail-open, тогда как принцип `golang-security` — «fail closed, never open». `RequireTenantMember` тоже не спасёт: без middleware claims в контекст не кладутся, а он при отсутствии claims пропускает запрос дальше ([pkg/auth/tenant.go:26-30](pkg/auth/tenant.go#L26-L30)).

Blast radius — тотальный: чтение/изменение инцидентов, политик, расписаний любого тенанта анонимно. Триггерится конфигурацией, а не активным эксплойтом, поэтому в шкале аудита это **major** (по DREAD — High; «critical при мисконфиге»).

**Фикс.** Сделать аутентификацию обязательной: если `KEYCLOAK_JWKS_URL` пуст — `os.Exit(1)` на старте (fail-closed), а «выключенный auth» допускать только под явным флагом `AUTH_DISABLED=true` для локалки, с громким `logger.Warn`. Идеально — вынести wiring middleware в общий `pkg/httpserver` (ср. F4 области 1), чтобы нельзя было «забыть» защиту в новом сервисе.

---

### S2 — SSRF через подконтрольный тенанту URL вебхука Mattermost — **major** (Medium)

Поток данных: тенант-админ → `PUT /api/schedules/v1/tenants/{slug}/notification-config` → сохраняется `MattermostWebhookURL` → notification-консьюмер читает его и делает HTTP POST.

На записи валидации хоста нет — берётся любая непустая строка:

```go
if patch.MattermostWebhookURL != nil && *patch.MattermostWebhookURL != "" {
    cur.MattermostWebhookURL = *patch.MattermostWebhookURL   // scheduling/handler.go:705-707
}
```

На отправке `webhookURLUsable` проверяет только синтаксис (scheme/host/path непустые), но **не ограничивает адрес**:

```go
u, err := url.Parse(raw)
if err != nil || u.Scheme == "" || u.Host == "" { return false }
return u.Path != "" && u.Path != "/"   // notifier.go:321-325 — никакого allowlist/блокировки приватных IP
```

Затем `n.mattermost.Send(ctx, cfg.MattermostWebhookURL, ...)` делает `POST` на этот URL ([notifier.go:152](services/notification/internal/notifier/notifier.go#L152)). Аутентифицированный тенант-админ может указать `http://169.254.169.254/latest/meta-data/...`, `http://localhost:8082/...` или адрес внутреннего k8s-сервиса и заставить notification-под обращаться во внутреннюю сеть от своего имени (SSRF). Тело POST контролируется частично (текст уведомления), ответ наружу не возвращается (blind SSRF), но доступна разведка внутренней сети и обращение к metadata-эндпоинтам облака.

Severity — Medium: нужен валидный тенант-админ, эксфильтрация ограничена (blind), но пересекается граница к инфраструктуре.

**Фикс.** На записи валидировать URL: только `https`, хост — по allowlist доменов Mattermost или хотя бы резолв + блокировка приватных/loopback/link-local диапазонов (RFC 1918, `169.254.0.0/16`, `::1`, `fc00::/7`). На отправке — тот же фильтр (defense in depth) + HTTP-клиент с запретом редиректов на приватные адреса. Готовые решения: пакет проверки SSRF или кастомный `DialContext` с проверкой `net.IP`.

---

### S3 — Admin-key: небезопасное сравнение и неограниченный god-key — **major** (Medium)

```go
if adminKey != "" && r.Header.Get("X-Admin-Key") == adminKey {   // pkg/auth/auth.go:102
    next.ServeHTTP(w, r.WithContext(WithMethod(r.Context(), MethodService)))
    return
}
```

Две проблемы:

1. **Сравнение секрета через `==`** — не constant-time, короткое замыкание на первом несовпавшем байте утекает тайминг (CWE-208). На практике эксплуатация по сети сложна (джиттер ≫ разницы в наносекундах), но фикс тривиален, а скил прямо относит это к Medium. Нигде в коде нет `crypto/subtle` (`grep` пуст).
2. **Admin-key — это god-key.** При совпадении запрос проходит как `MethodService` **без claims**, а `RequireTenantMember`/`RequireTenantAdmin` при отсутствии claims пропускают запрос ([tenant.go:26-30](pkg/auth/tenant.go#L26-L30)). То есть один статический ключ даёт неограниченный доступ ко **всем тенантам и всем эндпоинтам**, и он ещё и общий для межсервисных вызовов (`SCHEDULING_ADMIN_KEY`, `INCIDENT_ADMIN_KEY`). Концентрация доверия максимальная: утечка одного ключа = полный кросс-тенант компромисс.

**Фикс.**
- Сравнение: `subtle.ConstantTimeCompare([]byte(got), []byte(adminKey)) == 1` (с предварительной проверкой непустоты).
- Снизить blast radius: разные ключи на сервис/назначение, ротация, хранение в secret-manager; в идеале — заменить статический admin-key на mTLS или сервисные JWT с ограниченной областью (только нужные S2S-маршруты), а не «обход всего».

---

### S4 — Неполная валидация JWT — **major** (Medium)

```go
mc := jwt.MapClaims{}
token, err := jwt.ParseWithClaims(raw, &mc, kf.Keyfunc)   // pkg/auth/auth.go:114
if err != nil || !token.Valid { ... }
```

Подпись проверяется по JWKS (хорошо), но `ParseWithClaims` вызывается **без опций парсера**, поэтому отсутствуют:

- **`jwt.WithAudience(...)`** — `aud` не проверяется. Любой токен, выпущенный тем же realm Keycloak для **другого клиента/аудитории**, принимается всеми сервисами. В мультиклиентском realm это межклиентское повышение доступа.
- **`jwt.WithIssuer(...)`** — `iss` не сверяется с ожидаемым realm.
- **`jwt.WithValidMethods([]string{"RS256"})`** — нет allowlist алгоритмов. Защита от alg-confusion держится лишь на том, что `keyfunc` вернёт типизированный RSA-ключ (HMAC-метод не примет `*rsa.PublicKey`), но это неявно; явный allowlist обязателен по best practice.
- **`jwt.WithExpirationRequired()`** — golang-jwt/v5 проверяет `exp` только если клейм присутствует. Токен **без** `exp` пройдёт как валидный и будет действовать вечно.

Совокупно — ослабленная проверка подлинности на главном trust boundary.

**Фикс.** Передать опции в парсер:

```go
token, err := jwt.ParseWithClaims(raw, &mc, kf.Keyfunc,
    jwt.WithValidMethods([]string{"RS256"}),
    jwt.WithIssuer(expectedIssuer),
    jwt.WithAudience(expectedAudience),
    jwt.WithExpirationRequired(),
)
```

`expectedIssuer`/`expectedAudience` вынести в конфиг.

---

### S5 — Схема JWKS URL не форсится на HTTPS — **minor** (Low)

`keyfunc.NewDefault([]string{jwksURL})` ([auth.go:90](pkg/auth/auth.go#L90)) тянет ключи по тому URL, что задан в конфиге, без проверки, что это `https`. Если `KEYCLOAK_JWKS_URL` указан как `http://...` (что вероятно в in-cluster конфигурации к Keycloak), MITM в сети между сервисом и Keycloak может подменить набор ключей и, как следствие, выпускать принимаемые токены. В пределах доверенной кластерной сети риск низкий, но это снимаемая гарантия.

**Фикс.** Валидировать на старте, что схема `KEYCLOAK_JWKS_URL` == `https` (или явно разрешать http только при `AUTH_INSECURE=true` для локалки).

---

### S6 — Нет rate-limiting на входе — **minor** (Low)

Rate-limiter в проекте есть, но только **исходящий** — на уведомления ([notification/ratelimit](services/notification/internal/ratelimit/ratelimit.go)). Входящие эндпоинты (вебхуки ingestion, auth-защищённые API) не ограничены по частоте. Тело ограничено 4 МБ (хорошо), но число запросов — нет: возможен флуд webhook-эндпоинта или перебор `X-Webhook-Token`. Токены генерируются `crypto/rand` (256 бит), поэтому брутфорс практически нереален — отсюда Low; основной риск — ресурсное исчерпание.

**Фикс.** Per-IP/per-token rate limit на ingestion (например, `golang.org/x/time/rate` или middleware на Redis) и базовые таймауты (частично уже есть, ср. F4 области 1).

> Минорно и не выделяю в отдельную находку: разбор `Authorization` через `strings.TrimPrefix(h, "Bearer ")` ([auth.go:111](pkg/auth/auth.go#L111)) регистрозависим и принимает токен без схемы `Bearer`. Это не уязвимость (пустой результат → 401), но стоит привести к строгому разбору схемы.

---

## Что сделано хорошо (для контекста)

- **SQL-инъекций нет** — сплошная параметризация, динамические фильтры строят только плейсхолдеры `$N` (детально в области 3). `ORDER BY` не берётся из ввода.
- **Токены вебхуков — `crypto/rand`**, 256 бит, хранятся как SHA-256-хеш, сравниваются через lookup (не `==`) ([scheduling/handler.go:743-748](services/scheduling/internal/handler/handler.go#L743-L748)). `math/rand` в чувствительных путях нет.
- **Тело вебхука ограничено** `http.MaxBytesReader(..., 4<<20)` во всех webhook-хендлерах ([handler.go:88](services/ingestion/internal/handler/handler.go#L88)) — защита от memory-DoS.
- **Секреты не хардкодятся** (всё через env) и **не логируются** — в лог-вызовах нет полей secret/password/token (проверено grep'ом); `MattermostWebhookURL` маскируется для не-service вызовов ([handler.go:732](services/scheduling/internal/handler/handler.go#L732)).
- **Авторизация по группам защищена от prefix-confusion**: `IsMember` требует точного совпадения `/slug` либо префикса `/slug/`, `IsAdmin` — точного `/slug/admins` ([auth.go:69-86](pkg/auth/auth.go#L69-L86)); `/acme` не матчит `/acme-corp`.
- **Безопасные type-assertion'ы** (comma-ok) в разборе claims (`mapStr`, `mapStrSlice`) — нет паник на неожиданных типах JWT-клеймов (`golang-safety`).
- **Webhook token не найден → 401** (fail-closed на этом конкретном пути) ([tenant.go:38-41](services/ingestion/internal/middleware/tenant.go#L38-L41)).

---

## Рекомендованный порядок исправлений

1. **S1** — сделать auth fail-closed (обязательный JWKS, явный флаг для отключения): закрывает риск полного анонимного доступа при мисконфиге.
2. **S4** — добавить опции валидации JWT (`aud`/`iss`/`alg`/`exp`): укрепляет главный trust boundary, изменение локальное в `pkg/auth`.
3. **S2** — allowlist/блокировка приватных адресов для Mattermost webhook URL (на записи и отправке): закрывает SSRF.
4. **S3** — `subtle.ConstantTimeCompare` для admin-key + план на сужение его полномочий/ротацию.
5. **S5 + S6** — форс https для JWKS, входной rate-limit (по мере ужесточения).

> Кросс-ссылки: S1 опирается на общий `pkg/httpserver` (F4 области 1), где recovery (E1) и auth-wiring логично собрать в один не-забываемый барьер. S3 пересекается с областью 1 (admin-key как общий S2S-механизм — F3). Запуск `gosec ./...` и `govulncheck ./...` в CI отнесён в область CI/инструментов.
