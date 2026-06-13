## 1. pkg/events — канонические контракты (F1)

- [x] 1.1 F1 — создать `pkg/events/events.go` с типами `EscalationTriggered`,
  `EscalationExhausted`, `IncidentChanged`; JSON-теги перенести 1:1 из
  `services/escalation/internal/publisher/publisher.go:13-30`,
  `services/incident/internal/publisher/publisher.go:11-18`; package-комментарий
  со ссылкой на routing-ключи `pkg/amqp`.

## 2. escalation (F1)

- [x] 2.1 F1 — `escalation/internal/publisher/publisher.go:13-30`: удалить
  `TriggeredEvent`/`ExhaustedEvent`; методы `PublishTriggered`/`PublishExhausted`
  и `Noop.*` принимают `events.EscalationTriggered`/`events.EscalationExhausted`.
- [x] 2.2 F1 — `escalation/internal/escalator/escalator.go:31-32,117,228`:
  интерфейс `Publisher` и call-sites → `events.EscalationTriggered`/`Exhausted`.
- [x] 2.3 F1 — `escalation/internal/consumer/consumer.go:14-21,64,104`: удалить
  приватный `incidentPayload`, `Unwrap` в `events.IncidentChanged`.

## 3. incident (F1, F8, N4)

- [x] 3.1 F1 — `incident/internal/publisher/publisher.go:11-18`: удалить
  `IncidentEvent`; методы `PublishCreated`/`PublishUpdated` принимают
  `events.IncidentChanged`.
- [x] 3.2 F1 — `incident/internal/handler/handler.go:43-44,195` и
  `incident/internal/consumer/consumer.go:34-35,185,227`: интерфейсы `Publisher`
  и call-sites → `events.IncidentChanged`.
- [x] 3.3 F8 — `incident/internal/domain/incident.go:13-18`: удалить
  `AlertStatus`/`AlertFiring`/`AlertResolved`; поле `IncidentAlert.Status` →
  `pkg/domain.AlertStatus`.
- [x] 3.4 N4 — `incident/internal/consumer/consumer.go:176`,
  `incident/internal/handler/handler.go:235`: `incdomain.AlertFiring` →
  `domain.AlertStatusFiring` (согласование имён статусов между пакетами).

## 4. notification (F1)

- [x] 4.1 F1 — `notification/internal/notifier/notifier.go:19-36`: удалить
  `TriggeredEvent`/`ExhaustedEvent`; `NotifyTriggered`/`NotifyExhausted`/
  `dispatchToContact`/`mattermostText` → `events.EscalationTriggered`/`Exhausted`.
- [x] 4.2 F1 — `notification/internal/consumer/consumer.go:71-82`: `Unwrap`/декод
  в `events.EscalationTriggered`/`events.EscalationExhausted`.

## 5. Тесты

- [x] 5.1 Обновить юнит/интеграционные тесты escalation/incident/notification,
  ссылающиеся на удалённые локальные типы (`publisher.*Event`, `notifier.*Event`,
  `incidentPayload`, `incdomain.AlertFiring/AlertResolved`) → на `events.*` /
  `pkg/domain.AlertStatus`.

## 6. ADR

- [x] 6.1 `docs/adr/0014-pkg-events-bus-contracts.md` — «общий тип контракта vs
  параллельные копии» (продолжение ADR-0010); Change: extract-bus-contracts.

## 7. Верификация (Definition of Done)

- [x] 7.1 `go build ./...` и `go vet ./...` во всех модулях `go.work`.
- [x] 7.2 `go test ./...` во всех затронутых модулях (escalation/incident/notification/pkg).
- [x] 7.3 `go mod tidy` в задетых модулях — без диффа; `golangci-lint run` — 0 new issues.
- [x] 7.4 Контракт: убедиться, что продюсер и консьюмер каждого события
  (`escalation.*`, `incident.*`) ссылаются на один тип из `pkg/events`.
- [x] 7.5 `/opsx:verify` → `/opsx:archive --skip-specs` (no-delta infra-рефактор).
- [x] 7.6 Обновить `docs/audit/00-roadmap.md` (дашборд + строка CH05 → ✅ done).
