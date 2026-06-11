# ADR-0010: Самодостаточный payload событий: notification не делает синхронных вызовов за содержимым

- Status: Accepted
- Date: 2026-06-06
- Change: sre-oncall-platform (commit c7941dd); расширено в enrich-notifications-drop-zabbix (commit 007c021, 2026-06-10)
- Affected: services/escalation, services/notification, services/incident, pkg/amqp

## Context

Notification находится в конце событийной цепочки ([ADR-0002](0002-rabbitmq-event-bus.md)) и должен отправить дежурному email/Mattermost с контекстом, достаточным для решения без открытия системы (заголовок, severity, статус, ссылка). Изначально payload `escalation.triggered` нёс только идентификаторы и tier — письмо сводилось к «Escalation tier N», хотя `incident.created` уже публикует нужные поля.

## Options considered

- **Обогащение события на стороне escalation** — escalation сохраняет `incident_title`, `incident_severity`, `incident_status` из payload `incident.created` (а `oncall_user_id`/`oncall_username` резолвит у scheduling) и кладёт всё в `escalation.triggered`. Production-путь обходится без новых синхронных зависимостей.
- **Notification сам запрашивает incident при отправке** — добавляет sync-зависимость на каждый dispatch и противоречит принципу «payload события самодостаточен». Отклонено.
- **Запрашивать у incident в момент триггера** — лишний вызов в горячем пути эскалации; статус и так известен. Отклонено.

## Decision

События — основной транспорт данных: payload `escalation.triggered` самодостаточен (дежурный + title/severity/status инцидента), и notification строит содержимое уведомления только из события. Единственный sync-fallback — редкий ручной путь привязки политики, где escalation делает один GET к incident с сервисным ключом ([ADR-0009](0009-service-auth-via-admin-key.md)); при сбое поля остаются пустыми, привязка не блокируется. Ссылка в уведомлении — deep link фронтенда (`{FRONTEND_BASE_URL}/{tenant}/incidents?incident={id}`), а не API-эндпоинт. Для tenant-конфигурации (Mattermost webhook URL) notification кэширует ответ scheduling in-process (LRU, TTL 5 минут).

## Consequences

- Эволюция payload только аддитивна: новые поля добавляются, существующие не переименовываются; потребители обязаны переживать отсутствие новых полей (деплой издателя и потребителя не требует синхронности).
- Данные в событии фиксируются на момент создания инцидента — рассинхронизация при последующих изменениях принята (severity и так не пересчитывается).
- Notification зависит от `FRONTEND_BASE_URL` в конфигурации; без него уведомления уходят без ссылки (warn-лог), доставка не блокируется.
- Будущие потребители событий должны получать данные тем же способом — через payload, а не синхронными вызовами в горячем пути.
