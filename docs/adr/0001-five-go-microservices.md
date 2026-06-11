# ADR-0001: Декомпозиция на пять Go-микросервисов с REST API за общим Ingress

- Status: Accepted
- Date: 2026-06-06
- Change: sre-oncall-platform (commit c7941dd)
- Affected: services/ingestion, services/incident, services/scheduling, services/escalation, services/notification, deploy/k8s

## Context

Новая мультитенантная платформа дежурств для SRE-команд на Kubernetes, без миграции с легаси. У каждого домена своя модель данных, профиль нагрузки и внешние зависимости: `ingestion` испытывает пики во время волн алертов; `notification` требует изолированного rate-limiting; `escalation` управляется таймерами. Все сервисы написаны на Go.

## Options considered

- **Пять микросервисов** (`ingestion`, `incident`, `scheduling`, `escalation`, `notification`) — независимое масштабирование и деплой; требует шину событий между сервисами.
- **Монолит** — проще для старта, но усложняет независимое масштабирование и делает необязательной единую шину событий, что ломает изоляцию сервисов. Отклонён.
- **Отдельный сервис API-шлюза** для внешнего API — отклонён для v1: достаточно маршрутизации путей в Ingress.

## Decision

Платформа разбита на пять Go-микросервисов, каждый — отдельный Kubernetes Deployment. Каждый сервис предоставляет собственный REST API; Ingress (nginx) маршрутизирует `/api/ingestion/*`, `/api/incidents/*`, `/api/schedules/*`, `/api/escalations/*`, `/api/notifications/*` к соответствующему сервису без выделенного шлюза.

## Consequences

- Сервисы деплоятся и тестируются независимо; масштабирование per-domain.
- Объединение доменов потребовало шину событий (см. [ADR-0002](0002-rabbitmq-event-bus.md)) и дисциплину владения данными (см. [ADR-0003](0003-postgres-schema-per-service.md)).
- Любой новый внешний эндпоинт должен вписываться в схему путей `/api/{service}/...` и маршрутизацию Ingress.
- Появление выделенного API-шлюза остаётся возможным будущим изменением, но не предполагается в текущей архитектуре.
