# ADR-0003: Схема PostgreSQL на сервис в общем кластере

- Status: Accepted
- Date: 2026-06-06
- Change: sre-oncall-platform (commit c7941dd)
- Affected: services/ingestion, services/incident, services/scheduling, services/notification, services/escalation, pkg/db, pkg/migrate

## Context

Каждый из пяти сервисов ([ADR-0001](0001-five-go-microservices.md)) владеет собственной моделью данных, и границы владения нужно закрепить технически, не усложняя локальную разработку на k3s.

## Options considered

- **Схема на сервис в одном кластере PostgreSQL** — изоляция на уровне схем закрепляет границы владения без операционных издержек нескольких кластеров в разработке.
- **Отдельная база данных на сервис** — лучшая изоляция, но требует больше инфраструктуры при локальной разработке; можно мигрировать позже при необходимости. Отклонена для v1.

## Decision

Каждый сервис владеет своей PostgreSQL-схемой (`ingestion`, `incident`, `scheduling`, `notification`, `escalation`). Все схемы живут в одном кластере. Каждый сервис владеет своими файлами миграций (golang-migrate); CI запускает миграции в тестах.

## Consequences

- Сервис пишет и читает только собственную схему; межсервисный доступ к данным — через REST API или события, не через чужие таблицы. Например, `escalation` обновляет только свою `incident_escalation_states`, а ingestion не читает схему `scheduling` напрямую (lookup токенов — через Redis, см. [ADR-0005](0005-row-level-multitenancy.md)).
- Рассинхронизация схем между сервисами остаётся риском — митигируется миграциями per-service в CI.
- Переезд на отдельные БД per-service возможен позже без смены модели владения.
