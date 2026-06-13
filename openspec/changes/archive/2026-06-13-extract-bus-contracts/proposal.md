## Why

Payload-контракты шины событий продублированы независимыми Go-типами в каждом
сервисе. `escalation.triggered`/`escalation.exhausted` объявлены и у продюсера
(`escalation/internal/publisher`), и у консьюмера (`notification/internal/notifier`);
`incident.created`/`incident.updated` — как `IncidentEvent` у продюсера
(`incident/internal/publisher`) и как приватный `incidentPayload` у консьюмера
(`escalation/internal/consumer`). Это разные типы в разных модулях, поэтому
компилятор не подсвечивает рассинхронизацию: добавление поля у продюсера никак
не отражается у консьюмера. Сами комментарии в коде признают риск дрейфа
(«mirrors the escalation.triggered payload»). Параллельно `incident/internal/domain`
переобъявляет `AlertStatus`, дублируя канонический `pkg/domain.AlertStatus`, и
имена одного и того же статуса разъезжаются между пакетами
(`AlertFiring` vs `AlertStatusFiring`).

## What Changes

- Новый пакет `pkg/events` — единственный источник правды для payload'ов событий
  эскалации и инцидентов (F1):
  - `EscalationTriggered` (событие `escalation.triggered`),
  - `EscalationExhausted` (событие `escalation.exhausted`),
  - `IncidentChanged` (события `incident.created` / `incident.updated`).
- Удаление локальных дублей: `publisher.TriggeredEvent`/`ExhaustedEvent` (escalation),
  `notifier.TriggeredEvent`/`ExhaustedEvent` (notification), `publisher.IncidentEvent`
  (incident), приватный `consumer.incidentPayload` (escalation). Продюсеры и
  консьюмеры импортируют один тип из `pkg/events`.
- `incident/internal/domain` отказывается от собственного `AlertStatus`
  (`AlertFiring`/`AlertResolved`) в пользу канонического `pkg/domain.AlertStatus`
  (`AlertStatusFiring`/`AlertStatusResolved`) — закрывает F8 и часть N4
  (согласование имён статусов между пакетами). Инцидент-специфичный
  `domain.Status` (open/acknowledged/resolved) остаётся.
- JSON-теги канонических типов переносятся **байт-в-байт** из текущих структур.
  Формат сообщений на проводе НЕ меняется — это **НЕ BREAKING**: сообщения,
  уже лежащие в очередях, и сообщения от старых версий сервисов
  десериализуются как прежде.

## Capabilities

### New Capabilities
<!-- Новых продуктовых capability не вводится: это внутренний infra-рефактор
     транспортного контракта без изменения наблюдаемого поведения. -->

### Modified Capabilities
<!-- Нет. Wire-формат событий и поведение конвейера не меняются, поэтому
     дельт capability-спеков нет (архивация с --skip-specs, прецедент
     harden-auth-shell / extract-* инфра-чейнджей). -->

## Impact

- **Затронутые сервисы:**
  - `escalation` — publisher (продюсер `escalation.*`), consumer (консьюмер `incident.*`), escalator.
  - `incident` — publisher (продюсер `incident.*`), consumer, handler, internal/domain.
  - `notification` — notifier (консьюмер `escalation.*`), consumer.
  - `ingestion` — **не затронут** (`alert.received` уже несёт канонический
    `pkg/domain.Alert`, дубля нет — вне области F1).
- **События RabbitMQ:** `escalation.triggered`, `escalation.exhausted`,
  `incident.created`, `incident.updated` — типизация унифицирована через
  `pkg/events`; **wire-формат без изменений (НЕ BREAKING)**. `alert.received`
  не затрагивается.
- **pkg:** новый пакет `pkg/events` (в существующем модуле
  `github.com/sre-oncall/pkg`, без нового go.mod). `incident/internal/domain`
  теряет дублирующий тип `AlertStatus`.
- **ADR:** ADR-0014 фиксирует выбор «общий тип контракта vs параллельные копии»
  (область «версионирование событий» из правил репозитория; продолжение
  ADR-0010 о self-sufficient payloads).
- **Тесты:** обновляются юнит/интеграционные тесты escalation/incident/notification,
  ссылающиеся на удаляемые локальные типы.
- **Capability-спеки:** дельт нет (наблюдаемое поведение не меняется).
