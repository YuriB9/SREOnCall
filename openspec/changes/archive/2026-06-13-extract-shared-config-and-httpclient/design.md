## Context

Аудит (`docs/audit`) зафиксировал четыре связанных находки про дублирование
инфраструктурного кода S2S:

- **F3** — четыре почти одинаковых HTTP-клиента «GET к соседнему сервису с
  `X-Admin-Key`»; дрейф уже произошёл: escalation/schedclient нормализует `baseURL`
  (`strings.TrimRight`), а notification/schedclient — нет
  (`services/notification/internal/schedclient/client.go:31`).
- **F6** — хелперы env скопированы под разными именами (`getenv`/`envOr`/`getenvInt`/
  `envDurSec`); `notification.getenvInt` через `fmt.Sscanf` отвергает легитимный `0`.
- **E3** — клиенты сворачивают любой не-200 в безликий `fmt.Errorf`; вызывающий код не
  может `errors.Is(err, ErrNotFound)`. Усугублено тем, что `ErrNotFound` объявлен
  независимо в четырёх store-пакетах — это четыре разных значения.
- **P4** — все клиенты на дефолтном `http.Transport` (`MaxIdleConnsPerHost=2`).

Это чистый infra-рефактор в `pkg/*` (модуль `github.com/sre-oncall/pkg`), без нового
go.mod, в духе уже сделанных `pkg/events` (CH05) и `pkg/ssrf` (CH04).

## Goals / Non-Goals

**Goals:**

- Единый базовый S2S-клиент: нормализация `baseURL`, инъекция `X-Admin-Key`, маппинг
  статусов в общие sentinel'ы, тюнингованный общий `http.Transport`.
- Единый набор env-примитивов для `config.Load()`.
- Единственное значение `ErrNotFound`/`ErrConflict` на монорепо, чтобы `errors.Is`
  работал и внутри сервиса, и через сетевую границу.
- Сохранить наблюдаемое поведение (wire-формат, статусы хендлеров, контракт
  notification «404 → nil без ошибки»).

**Non-Goals:**

- Не менять контракты API и события RabbitMQ (не BREAKING).
- Не переписывать keycloak-клиент на admin-key-абстракцию (у него bearer-токены и
  form-POST) — ему достаётся только общий тюнингованный Transport (P4).
- Не трогать SSRF-guarded dialer mattermost из CH04 — только тюнинг idle-conns.
- Не выносить бутстрап `main()` и middleware (F4/F10/F5/F9) — это CH10.

## Decisions

### 1. `pkg/errs` — общие sentinel'ы (E3)

```go
package errs
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
```

Стоковые `store.ErrNotFound`/`ErrConflict` в incident/escalation/scheduling/notification
становятся **алиасами** на эти значения (`var ErrNotFound = errs.ErrNotFound`). Все
существующие `errors.Is(err, store.ErrNotFound)` и
`OverrideConflictError.Is(target == ErrConflict)` продолжают работать, но значение теперь
одно на монорепо — клиент, вернувший `errs.ErrNotFound`, отличим вызывающим кодом от
технической ошибки. Альтернатива «не алиасить, оставить 4 значения» отклонена — именно
она и есть корень E3.

### 2. `pkg/httpclient` — базовый S2S-клиент (F3 + E3 + P4)

```go
package httpclient

// общий тюнингованный транспорт, разделяемый всеми клиентами (P4)
func New(baseURL, adminKey string) *Client       // TrimRight baseURL, общий Transport
func (c *Client) GetJSON(ctx, path string, out any) error
```

`GetJSON`: строит запрос на `baseURL+path`, ставит `X-Admin-Key` (если непустой),
`Do`, затем маппинг статуса — `404→errs.ErrNotFound`, `409→errs.ErrConflict`, прочее
не-2xx → `fmt.Errorf("...: %w", ...)`, 2xx → `json.NewDecoder(...).Decode(out)`.
Сервисные клиенты остаются (`schedclient`, `incclient`) и описывают только эндпойнт+DTO,
оборачивая `*httpclient.Client` («accept interfaces, return structs» —
`golang-structs-interfaces`). notification сохраняет контракт «нет конфига → nil»:
`if errors.Is(err, errs.ErrNotFound) { return nil, nil }`.

Общий Transport — пакетная переменная (`Clone()` от `http.DefaultTransport` с поднятыми
`MaxIdleConns`/`MaxIdleConnsPerHost`/`IdleConnTimeout`). keycloak берёт `*http.Client`
с этим транспортом через `httpclient.NewStdClient(timeout)`.

### 3. `pkg/config` — env-примитивы (F6)

```go
package config
func String(key, def string) string
func Int(key string, def int) int                 // strconv.Atoi; пустая → def, иначе парсинг
func DurationSeconds(key string, def time.Duration) time.Duration
```

`Int` через `strconv.Atoi` (а не `Sscanf`) принимает легитимный `0` — чинит баг
`notification.getenvInt`. Сервисные `config.Load()` остаются, но зовут эти примитивы.

## Risks / Trade-offs

- **Алиасинг sentinel'ов** — риск минимален: `errors.Is`/`Is`-методы инвариантны к тому,
  где объявлено значение; покрыто существующими тестами хендлеров + `go build` всех
  модулей.
- **Изменение поведения `Int` на `0`** (F6) — теперь `RATE_LIMIT_MAX=0` примет `0`
  вместо дефолта. Это и есть цель находки; дефолты при пустой переменной не меняются.
- **go.work-монорепо:** смена `pkg/*` затрагивает все сервисы → собираем и тестируем все
  модули, `go mod tidy` в каждом задетом.
- **Без дельты-спека** — наблюдаемое поведение продуктовых capability не меняется
  (прецедент `harden-auth-shell`, `extract-bus-contracts`): архив с `--skip-specs`.
