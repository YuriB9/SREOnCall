# ADR-0007: Эскалация через периодический опрос таблицы состояний

- Status: Accepted
- Date: 2026-06-06
- Change: sre-oncall-platform (commit c7941dd)
- Affected: services/escalation

## Context

Эскалация неподтверждённых инцидентов требует механизма таймеров, переживающего перезапуски сервиса, без распределённого крон-планировщика.

## Options considered

- **Периодический опрос собственной таблицы `incident_escalation_states`** — прост, аудируем, состояние в Postgres переживает рестарты.
- **Keyspace-уведомления Redis по TTL-истечению** — нестабильно (события могут теряться при перезапуске Redis) и связывает логику эскалации с Redis. Отклонено.

## Decision

Сервис `escalation` периодически (интервал настраивается, по умолчанию ~30 сек) опрашивает свою таблицу `incident_escalation_states` на записи с истёкшим `escalate_at`. При срабатывании обновляет `current_tier`/`escalate_at`, запрашивает scheduling за текущим дежурным и публикует обогащённое событие `escalation.triggered` на exchange `escalations` (см. [ADR-0010](0010-self-sufficient-event-payloads.md)).

## Consequences

- Задержка эскалации до интервала опроса — принята для сценариев дежурств и задокументирована; интервал управляется env-переменной.
- Состояние эскалации — обычные строки Postgres: аудируемо, тестируемо, без внешних таймер-систем.
- Notification потребляет только события exchange `escalations`; событие `incident.created` он не слушает — первый `escalation.triggered` (tier 1) покрывает первичное оповещение.
