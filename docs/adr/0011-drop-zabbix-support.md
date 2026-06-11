# ADR-0011: Вывод Zabbix из поддерживаемых источников алертов

- Status: Accepted
- Date: 2026-06-10
- Change: enrich-notifications-drop-zabbix (commit 007c021)
- Affected: services/ingestion, services/scheduling, services/incident, pkg/domain, frontend

## Context

Спека `alert-ingestion` требовала эндпоинт приёма Zabbix, но он не был реализован — при этом система позволяла создать webhook-токен с `source: zabbix`, которым некуда слать, а `zabbix` фигурировал в умолчаниях группировки и в UI.

## Options considered

- **Вывести Zabbix из поддержки** — удалить требование и `zabbix` из всех enum'ов; ничего не дореализовывать.
- **Дореализовать приём Zabbix** — отклонено решением владельца: источник не нужен.
- **Миграция-чистка существующих `zabbix`-строк в БД** — отклонена как необратимая операция ради косметики: данные не мешают и не создают дыр (эндпоинта-потребителя не существует).

## Decision

Поддерживаемые источники алертов — только `alertmanager` и `grafana`. Удалены: требование Zabbix из `alert-ingestion`, `zabbix` из enum источников токенов (scheduling + UI), zabbix-ветки умолчаний группировки (incident), константа `SourceZabbix` из `pkg/domain`. Существующие строки с `source='zabbix'` не мигрируются; отзыв токенов — ручная операция администратора.

## Consequences

- Создание токена с `source: zabbix` отклоняется валидацией (422).
- Возврат Zabbix — не «раз-удаление», а новое изменение с ADDED-требованием и реализацией приёма; REMOVED-дельта документирует этот путь.
- Добавление любого нового источника алертов означает синхронное расширение enum'ов в scheduling, incident, `pkg/domain` и UI плюс эндпоинт в ingestion — частичная поддержка источника (как было с Zabbix) недопустима.
