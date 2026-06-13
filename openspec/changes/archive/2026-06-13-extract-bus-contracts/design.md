## Context

Транспорт шины (`pkg/amqp`: `Envelope`, `Wrap`/`Unwrap`, имена exchange/queue/routing
в `topology.go`) уже централизован, но **payload-контракты** туда не вынесены.
Сейчас одно и то же тело события объявлено независимыми Go-типами в разных модулях:

- `escalation.triggered`/`exhausted`: `TriggeredEvent`/`ExhaustedEvent` в
  `escalation/internal/publisher/publisher.go` (продюсер) **и** в
  `notification/internal/notifier/notifier.go` (консьюмер);
- `incident.created`/`updated`: `IncidentEvent` в
  `incident/internal/publisher/publisher.go` (продюсер) **и** приватный
  `incidentPayload` в `escalation/internal/consumer/consumer.go` (консьюмер).

Поскольку это разные типы в разных модулях `go.work`, компилятор не ловит
рассинхронизацию JSON-полей между продюсером и консьюмером — синхронность
держится только на дисциплине. Дополнительно `incident/internal/domain`
переобъявляет `AlertStatus`, дублируя `pkg/domain.AlertStatus`, с расходящимися
именами констант (`AlertFiring` vs `AlertStatusFiring`).

Это área **F1** (major), **F8** (minor) и часть **N4** (имена статусов) из
`docs/audit/01-structure-layers.md` и `docs/audit/09-style-idiomatic.md`.

## Goals / Non-Goals

**Goals:**
- Один источник правды для payload'ов `escalation.*` и `incident.*` —
  пакет `pkg/events`. Продюсер и консьюмер каждого события импортируют один тип.
- Удалить все локальные дубли payload-структур.
- Убрать дублирующий `incident/internal/domain.AlertStatus` в пользу
  `pkg/domain.AlertStatus`; согласовать имена статусов между пакетами.
- Сохранить wire-формат событий байт-в-байт (обратная совместимость очередей).

**Non-Goals:**
- НЕ трогаем `alert.received` — он уже несёт канонический `pkg/domain.Alert`,
  дубля нет (вне F1).
- НЕ вводим zero-value `Unknown`/`Invalid` и не правим префиксы sentinel-строк —
  это остальные части N4, отнесённые к CH18.
- НЕ меняем `pkg/amqp` (`Envelope`/`Wrap`/`Unwrap`/топологию) — это CH07/CH13.
- НЕ меняем поведение конвейера, набор полей или их семантику.
- НЕ вводим версионирование/version-константы событий (нет потребителя; модель
  совместимости остаётся «self-sufficient payloads», ADR-0010).

## Decisions

### 1. Новый пакет `pkg/events` как источник правды payload-контрактов

Канонические типы в `pkg/events/events.go` (модуль `github.com/sre-oncall/pkg`,
нового go.mod не требуется):

```go
package events

// EscalationTriggered — тело события escalation.triggered.
type EscalationTriggered struct {
    IncidentID       string `json:"incident_id"`
    TenantID         string `json:"tenant_id"`
    TenantSlug       string `json:"tenant_slug"`
    Tier             int    `json:"tier"`
    OncallUserID     string `json:"oncall_user_id"`
    OncallUsername   string `json:"oncall_username"`
    IncidentTitle    string `json:"incident_title"`
    IncidentSeverity string `json:"incident_severity"`
    IncidentStatus   string `json:"incident_status"`
}

// EscalationExhausted — тело события escalation.exhausted.
type EscalationExhausted struct {
    IncidentID string `json:"incident_id"`
    TenantID   string `json:"tenant_id"`
    TenantSlug string `json:"tenant_slug"`
}

// IncidentChanged — тело событий incident.created и incident.updated.
type IncidentChanged struct {
    IncidentID string `json:"incident_id"`
    TenantID   string `json:"tenant_id"`
    TenantSlug string `json:"tenant_slug"`
    Status     string `json:"status"`
    Title      string `json:"title"`
    Severity   string `json:"severity"`
}
```

Имена типов выбраны без stutter (`events.EscalationTriggered`, а не `EscalationTriggeredEvent`)
и по смыслу события. `IncidentChanged` объединяет created/updated (различаются routing-ключом
из `pkg/amqp`, не телом).

**Альтернатива (отклонена):** оставить локальные типы и держать синхронность
ревью/тестами. Отклонено — именно эту дисциплину аудит и зафиксировал как
системный риск (major F1): компилятор молчит при дрейфе.

**Альтернатива (отклонена):** положить контракты в `pkg/amqp` рядом с routing-ключами.
Отклонено — `pkg/amqp` отвечает за транспорт; смешивание с доменными payload'ами
ухудшит границы и усложнит будущий CH07/CH13. Отдельный `pkg/events` чище.

### 2. JSON-теги переносятся 1:1 — изменение НЕ BREAKING

Все три копии `TriggeredEvent`/`ExhaustedEvent` идентичны по полям и JSON-тегам;
`IncidentEvent` и `incidentPayload` идентичны. Канонические типы получают те же теги,
поэтому байтовое представление на проводе не меняется. Сообщения в очередях и от
старых версий сервисов десериализуются без изменений.

### 3. F8/N4 — единый `pkg/domain.AlertStatus`

`incident/internal/domain` удаляет `AlertStatus`/`AlertFiring`/`AlertResolved`.
Поле `IncidentAlert.Status` становится `pkg/domain.AlertStatus`; call-sites
(`incident/consumer.go`, `incident/handler.go`) используют `domain.AlertStatusFiring`.
Инцидент-специфичный `domain.Status` (open/acknowledged/resolved) остаётся —
он не дублирует ничего.

## Risks / Trade-offs

- **[Случайное изменение JSON-тега при переносе → молчаливый разрыв контракта]**
  → Переносим теги дословно; верификация: grep по полям до/после, и проверка
  «продюсер и консьюмер каждого события ссылаются на один `events.*`-тип»
  (раздел проверок DoD). Интеграционный e2e (при наличии) гоняет реальный round-trip.
- **[Связанность: все сервисы теперь зависят от `pkg/events`]** → приемлемо;
  это и есть цель (единый контракт). `pkg/events` не тянет внешних зависимостей.
- **[Затрагиваются `main.go`/интерфейсы продюсеров]** → сигнатуры методов
  publisher и интерфейсы `Publisher` меняют тип аргумента; правки механические,
  ловятся компилятором во всех модулях `go.work`.

## Migration Plan

Чисто компиляционный рефактор Go-типов; миграции БД и схем нет.

1. Добавить `pkg/events`.
2. Перевести продюсеры/консьюмеры/интерфейсы на `events.*`, удалить локальные дубли.
3. Перевести `incident/internal/domain` на `pkg/domain.AlertStatus`.
4. Обновить тесты.
5. `go build/vet/test ./...` во всех модулях; `golangci-lint`.

**Совместимость при деплое:** wire-формат не меняется → порядок выката сервисов
не важен, сообщения в очередях остаются валидными. **Откат:** обратный revert
ветки безопасен (нет персистентных изменений).

## Open Questions

Нет.
