## Why

Инфраструктурный код продублирован по сервисам и уже начал расходиться: четыре почти
одинаковых S2S-HTTP-клиента (один не нормализует `baseURL` → trailing slash в
`SCHEDULING_URL` ломает notification, но не escalation), хелперы чтения env скопированы
под разными именами в каждый `config`, sentinel `ErrNotFound` объявлен независимо в
четырёх store-пакетах (это четыре разных значения — `errors.Is` через сетевую границу
невозможен в принципе), а все клиенты ходят на дефолтном `http.Transport`
(`MaxIdleConnsPerHost=2`) → черн соединений под конкуренцией. Закрываем находки аудита
F3, F6, E3, P4 общими `pkg/*`-пакетами до того, как на эти клиенты обопрутся CH07/CH10.

## What Changes

- **Новый `pkg/config`** (F6): примитивы `String`/`Int`/`DurationSeconds` вместо
  скопированных `getenv`/`envOr`/`getenvInt`/`envDurSec`. Заодно чинится
  `notification.getenvInt`, который через `Sscanf` отвергал легитимный `0`.
- **Новый `pkg/errs`** (E3): `ErrNotFound`, `ErrConflict` — единственные значения на
  монорепо. Стоковые `store.ErrNotFound`/`ErrConflict` становятся алиасами на них.
- **Новый `pkg/httpclient`** (F3 + E3 + P4): базовый GET-JSON-клиент с нормализацией
  `baseURL`, инъекцией `X-Admin-Key`, тюнингованным общим `http.Transport` и маппингом
  статусов в sentinel'ы (`404→ErrNotFound`, `409→ErrConflict`, прочее → обёртка `%w`).
- **Перевод S2S-клиентов** escalation/schedclient, escalation/incclient,
  notification/schedclient на `pkg/httpclient`; scheduling/keycloak — на общий
  тюнингованный `*http.Client` (P4).
- **Перевод `config.Load()`** всех пяти сервисов на примитивы `pkg/config`.
- Контракты API и события RabbitMQ **не меняются** — рефактор. **Не BREAKING.**

## Capabilities

### New Capabilities
<!-- Нет: кросс-каттинг infra-рефактор в pkg/*, продуктовой capability не вводит. -->

### Modified Capabilities
<!-- Нет: наблюдаемое поведение продуктовых capability не меняется (как harden-auth-shell, extract-bus-contracts). Дельта-спека не требуется. -->

## Impact

- **Затронутые сервисы:** все пять (escalation, notification, scheduling, incident,
  ingestion) — на уровне `config.Load()` и S2S-клиентов. incident/ingestion затронуты
  только конфигом; escalation/notification/scheduling — конфигом и клиентами.
- **События RabbitMQ:** не меняются.
- **Схема БД / миграции:** нет.
- **Новые пакеты:** `pkg/config`, `pkg/errs`, `pkg/httpclient` (модуль
  `github.com/sre-oncall/pkg`, без нового go.mod).
- **Совместимость:** wire-формат, JSON-теги DTO и поведение хендлеров не меняются;
  `errors.Is(err, store.ErrNotFound)` и `OverrideConflictError.Is` продолжают работать
  (алиасы на общие значения). **Не BREAKING.**
- **mattermost-диспетчер** сохраняет SSRF-guarded dialer из CH04; добавляется лишь
  тюнинг idle-соединений.
