# Why

Сверка спецификаций с кодом (`docs/openspec-code-verification.md`, раздел 7, P1) выявила функциональные долги: уведомления дежурному не содержат обязательных по спеке данных — события `escalation.triggered` несут только идентификаторы и tier, поэтому email состоит из «Escalation tier N» без заголовка, severity, статуса и ссылки на инцидент, хотя `incident.created` уже публикует эти поля. Приём вебхуков Zabbix, требуемый спекой `alert-ingestion`, не реализован вовсе — при этом система позволяет создать webhook-токен с `source: zabbix`, которым некуда слать. Решение: Zabbix из спецификаций и enum'ов убирается (а не дореализуется). Попутно во фронтенде остались мёртвые хуки `useOnCallNow`/`useScheduleWindow`, второй из которых обращается к эндпоинту с неподдерживаемыми параметрами — ловушка для будущего переиспользования.

## What Changes

- **Обогащение событий эскалации**: `escalation.triggered` дополняется полями `incident_title`, `incident_severity`, `incident_status`; escalation сохраняет их из события `incident.created` (а при ручной привязке политики — запрашивает у incident-сервиса с сервисным ключом из изменения `fix-p0-delivery-and-filters`).
- **Полноценное содержимое уведомлений**: email получает заголовок, severity, статус, ссылку на инцидент в веб-интерфейсе и временную метку; Mattermost-сообщение — ID, severity, заголовок, статус и ссылку. Ссылка строится из настраиваемого базового URL фронтенда (deep link `/{tenant}/incidents?incident={id}`), что заменяет прежнюю формулировку «ссылка на эндпоинт API инцидента».
- **BREAKING — удаление Zabbix из контрактов**: требование «Приём вебхуков Zabbix» удаляется из `alert-ingestion`; `source` webhook-токенов сужается до `alertmanager | grafana`; правила группировки и их умолчания — только для двух источников; `zabbix` удаляется из enum'ов бэкенда (`scheduling`, `incident`, `pkg/domain`) и фронтенда.
- **Чистка фронтенда**: удаление неиспользуемых хуков `useOnCallNow` и `useScheduleWindow` из `frontend/src/api/schedules.ts`.

## Capabilities

### New Capabilities

<!-- Новых capability не вводится. -->

### Modified Capabilities

- `alert-ingestion`: удаляется требование «Приём вебхуков Zabbix» (приём не реализован, источник выводится из поддержки).
- `escalation-policies`: требование «Срабатывание эскалации по таймауту» — событие `escalation.triggered` дополняется полями `incident_title`, `incident_severity`, `incident_status` (дельта строится поверх изменений `fix-p0-delivery-and-filters` — это изменение применяется после него).
- `notification-dispatch`: «Потребление событий эскалации» — контракт payload с обогащёнными полями; «Отправка Email-уведомлений через SMTP» — ссылка на инцидент в веб-интерфейсе вместо «эндпоинта API», содержимое строится из payload без синхронных вызовов.
- `tenant-management`: «Управление вебхук-токенами для ingestion» — источники только `alertmanager` и `grafana`.
- `incident-management`: «Настройка группирующих лейблов per-source» — источники и умолчания только для `alertmanager` и `grafana`.

## Impact

- **Бэкенд**:
  - `services/escalation`: новые поля в `escalation_states` (миграция БД), заполнение из `incident.created` (`consumer.go`), включение в `TriggeredEvent` (`publisher`, `escalator.go`); fallback-запрос к incident при ручной привязке;
  - `services/notification`: `TriggeredEvent`, `notifier.go`, `dispatcher/email.go`, `dispatcher/mattermost.go` — содержимое сообщений; конфиг `FRONTEND_BASE_URL`;
  - `services/scheduling/internal/handler/handler.go:596` — enum источников без zabbix;
  - `services/incident/internal/store/store.go:495`, `internal/domain/incident.go:88` — правила группировки без zabbix;
  - `pkg/domain/alert.go` — удаление `SourceZabbix`.
- **Фронтенд**: `frontend/src/pages/TenantSettingsPage.tsx:19` (`VALID_SOURCES`), `frontend/src/api/schedules.ts` (мёртвые хуки).
- **Деплой**: `FRONTEND_BASE_URL` в configmap notification.
- **Данные/совместимость**: существующие webhook-токены с `source: zabbix` остаются в БД, но не проходят валидацию при создании новых; администраторам рекомендуется отозвать их вручную. Потребители `escalation.triggered` обратносовместимы: новые поля добавляются, старые сохраняются.
- **Зависимость**: применяется после `fix-p0-delivery-and-filters` (общий сервисный ключ; общая правка требования «Срабатывание эскалации по таймауту»).
