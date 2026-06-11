## Context

Алерт приходит в ingestion с токеном вебхука; Redis-индекс `oncall:tokens:{hash}` хранит `tenant_id`, в качестве которого scheduling записывает **slug** тенанта (`tokenindex.Set(hash, slug)`). Таким образом во всём событийном конвейере `tenant_id == slug`.

Consumer incident-сервиса (`handleFiring`) создаёт инцидент так:

```go
inc := &incdomain.Incident{
    TenantID: tenantID,
    Title:    alert.Title,
    Severity: string(alert.Severity),
    Status:   incdomain.StatusOpen,
}
```

Поле `TenantSlug` не заполняется, в БД колонка `tenant_slug` имеет `DEFAULT ''`, и событие `incident.created` публикуется с `tenant_slug: ""`. HTTP-путь (`PatchStatus`) корректно берёт slug из URL, поэтому `incident.updated` из API-пути не страдает — но `incident.updated`, публикуемый consumer'ом при авто-резолве, тоже уходит с пустым slug.

Последствия пустого slug:
- escalation (`AutoAssign` → `AssignPolicy`) сохраняет `EscalationState.TenantSlug = ""`; `schedclient.GetOnCall` строит URL `/api/schedules/v1//schedules/{id}/oncall` → ошибка; `escalation.triggered` публикуется с пустыми `oncall_user_id`/`oncall_username`;
- notification: `cache.Get("")` → `GET /api/schedules/v1/tenants//notification-config` → 404 → Mattermost пропускается; ссылка на инцидент `{base}//incidents?...` битая.

Fallback `tenant_slug → tenant(r)` уже существует в **ручном** пути привязки политики (`handler.AttachPolicy`), но не в consumer-пути.

## Goals / Non-Goals

**Goals:**
- `incident.created` и `incident.updated` из consumer-пути всегда содержат непустой `tenant_slug`.
- Авто-назначенная эскалация резолвит дежурного и публикует `escalation.triggered` с корректным `tenant_slug`.
- Уже существующие записи с пустым `tenant_slug` (инциденты, состояния эскалации) исправлены backfill'ом.
- Устойчивость к событиям от старой версии incident-сервиса (события в очереди на момент деплоя): escalation применяет fallback `tenant_slug → tenant_id`.

**Non-Goals:**
- Разделение понятий `tenant_id` (UUID) и `slug` в событийном конвейере — сейчас они совпадают по построению, и это изменение не пытается это менять.
- Каскадные исправления других расхождений из аудита (severity, source `prometheus`, дубль notification-config) — отдельные изменения.
- Изменения в notification и frontend — они начинают работать корректно автоматически.

## Decisions

**1. Источник slug в consumer'е — `tenant_id` события, без запроса в scheduling.**
`tenant_id` в конвейере уже является slug'ом (см. Context). Синхронный запрос в scheduling за slug добавил бы сетевую зависимость и точку отказа в hot-path обработки алертов ради значения, которое уже есть в сообщении. Если в будущем `tenant_id` станет UUID, это изменение локализовано в одном месте (`handleFiring`).

*Альтернатива (отклонена):* запрашивать тенант у scheduling по `tenant_id` — лишний RPC, нарушает принцип «consumer не делает синхронных вызовов», ничего не даёт при текущем тождестве id/slug.

**2. Fallback в escalation на стороне consumer-пути, симметрично ручному пути.**
В `consumer.handle`/`ProcessDelivery` (или в начале `AutoAssign`) — `if payload.TenantSlug == "" { payload.TenantSlug = payload.TenantID }`. Это защищает от событий старой версии incident-сервиса, оставшихся в очереди `incidents.escalation` на момент деплоя, и от любых будущих регрессий. Спека escalation уже допускает «события от старой версии» для notification — применяем тот же принцип.

**3. Backfill миграциями, без кода.**
- incident: `UPDATE incident.incidents SET tenant_slug = tenant_id WHERE tenant_slug = ''`.
- escalation: `UPDATE escalation.escalation_states SET tenant_slug = tenant_id WHERE tenant_slug = ''` (важно для активных эскалаций: монитор продолжит переходы по уровням и сможет резолвить дежурного без рестарта цикла).
Backfill корректен, потому что `tenant_id == slug` для всех существующих записей (оба значения приходят из одного Redis-индекса).

*Альтернатива (отклонена):* лениво чинить slug при чтении — размазывает костыль по коду, не чинит уже сохранённые состояния эскалации для монитора.

**4. Порядок деплоя не важен.**
Fallback в escalation делает его совместимым со старым incident; новый incident с заполненным slug совместим со старым escalation. Миграции идемпотентны (`WHERE tenant_slug = ''`).

## Risks / Trade-offs

- [Тождество `tenant_id == slug` неявное] → зафиксировать его комментарием в месте присваивания и в delta-спеке; при будущем переходе на UUID-теннанты изменение всплывёт в одном месте.
- [События с пустым slug, опубликованные до деплоя, уже потреблены escalation и сохранены с пустым slug] → закрывается backfill-миграцией escalation_states.
- [Backfill затронет «исторические» закрытые инциденты] → безопасно: меняется только пустое значение на корректное; история/аудит не искажаются.
- [Невозможно проверить slug на корректность в consumer'е] → принимаем `tenant_id` как есть; невалидный slug означал бы невалидный токен-индекс, что вне зоны этого изменения.

## Migration Plan

1. Накатить миграции (incident, escalation) — идемпотентны, выполняются при старте сервисов (`pkgmigrate.Run`).
2. Деплой incident и escalation в любом порядке.
3. Откат: код можно откатывать свободно; миграции отката не требуют (данные становятся строго корректнее).

## Open Questions

Нет.
