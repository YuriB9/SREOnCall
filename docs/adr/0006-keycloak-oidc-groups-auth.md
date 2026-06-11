# ADR-0006: Аутентификация через Keycloak OIDC; членство и роли из claim `groups`

- Status: Accepted
- Date: 2026-06-06
- Change: sre-oncall-platform (commit c7941dd); клиентская трактовка согласована в harden-auth-shell (commit 3701ff6, 2026-06-08)
- Affected: pkg/auth, все сервисы, frontend/src/auth

## Context

Платформе нужны SSO, управление пользователями и ролевая модель per-tenant. Keycloak уже предполагается как инфраструктурный компонент; выносить управление пользователями и SSO в платформу — дублирование. До появления пользователей нужен bootstrap-доступ (создание первого тенанта).

## Options considered

- **Keycloak OIDC + claim `groups`** — членство и роли кодируются группами Keycloak; мутации участников — только через Keycloak Admin UI.
- **Локальная таблица `tenant_memberships`** — требует синхронизации с Keycloak и дополнительного API управления; лишняя сложность при наличии OIDC. Отклонена.

## Decision

Все сервисы проверяют Bearer JWT, выданный Keycloak, через JWKS-эндпоинт. Общий пакет `pkg/auth` реализует stateless middleware: только валидация JWT и извлечение claims (`sub`, `preferred_username`, `name`, `email`, `groups`) в Go context — никакого IO. Маппинг: `sub` → `user_id`; группа `/team-slug` или любая `/team-slug/<подгруппа>` → членство в тенанте; `/team-slug/admins` → роль admin. Кэш пользователей (upsert в таблицу `users`) ведёт только scheduling. Локальный суперадмин — статичный ключ в Kubernetes Secret, заголовок `X-Admin-Key`, полностью обходит Keycloak и tenant-проверки (нужен для bootstrap; см. также [ADR-0009](0009-service-auth-via-admin-key.md)).

## Consequences

- Граница безопасности — на бэкенде; клиентские гарды фронтенда — только UX.
- Groups-маппер обязан присутствовать и в `access_token` (читает бэкенд), и в `id_token` (читает фронтенд) — скрытая конфигурационная связка, зафиксированная в harden-auth-shell; клиентский `parseGroups` обязан зеркалировать серверные `IsMember`/`IsAdmin` (префиксные подгруппы — членство).
- При недоступности Keycloak валидация JWT падает → сервисы возвращают 401; это принято, так как Keycloak высокодоступен отдельно от платформы.
- Управление составом команд не реализуется в платформе — только чтение через Keycloak Admin API (client credentials).
