# Why

Сверка спецификаций с кодом (`docs/openspec-code-verification.md`, раздел 2) выявила четыре P0-дефекта, два из которых полностью ломают доставку уведомлений в k8s-деплое — ядро ценности on-call-платформы. Межсервисные вызовы escalation→scheduling и notification→scheduling не проходят JWT-авторизацию (клиенты не отправляют ни Bearer-токен, ни `X-Admin-Key`, а в k8s `KEYCLOAK_JWKS_URL` включён у scheduling) — эскалация публикует события с пустым `oncall_user_id`, и дежурный не получает ни email, ни Mattermost. Даже после починки авторизации notification получает **маскированный** `mattermost_webhook_url` (тот же эндпоинт, что и UI) и не может отправить сообщение. Дополнительно: выбор двух статусов в фильтре инцидентов возвращает пустую таблицу (фронт шлёт `status=open,acknowledged`, бэкенд сравнивает строгим равенством), а сохранение формы настроек уведомлений с пустым полем URL затирает сохранённый вебхук.

## What Changes

- **Сервисная авторизация (баг 2.1)**: `escalation/internal/schedclient` и `notification/internal/schedclient` начинают передавать сервисный ключ `X-Admin-Key` (уже поддерживается `pkg/auth.Middleware`) во всех запросах к scheduling; ключ поступает из конфигурации сервиса и k8s Secret.
- **Немаскированный конфиг для notification (баг 2.2)**: scheduling отдаёт `mattermost_webhook_url` без маскирования, когда запрос аутентифицирован сервисным ключом; маскирование сохраняется для всех пользовательских (JWT) запросов.
- **Мульти-статусный фильтр инцидентов (баг 2.3)**: параметр `status` в `GET /api/incidents/v1/{tenant}/incidents` принимает список через запятую (`?status=open,acknowledged`), бэкенд фильтрует через `IN (...)`.
- **Защита webhook URL при сохранении (баг 2.4)**: PUT конфигурации уведомлений тенанта трактует пустой/отсутствующий `mattermost_webhook_url` как «оставить текущее значение»; фронтенд не отправляет поле, если оно не заполнялось. Явная очистка URL выполняется отдельным выраженным действием.

## Capabilities

### New Capabilities

<!-- Новых capability не вводится. -->

### Modified Capabilities

- `escalation-policies`: требование «Срабатывание эскалации по таймауту» уточняется — запросы к scheduling за текущим дежурным выполняются с сервисной аутентификацией; сбой авторизации не должен приводить к тихой публикации события без дежурного.
- `notification-dispatch`: добавляется требование о получении per-tenant конфигурации из scheduling с сервисной аутентификацией и немаскированным `mattermost_webhook_url`.
- `tenant-management`: требование «Конфигурация уведомлений тенанта» уточняется — маскирование URL применяется только к пользовательским запросам; сервисные запросы получают полный URL; PUT с пустым `mattermost_webhook_url` сохраняет существующее значение.
- `incident-management`: требование «Список и фильтрация инцидентов» уточняется — фильтр `status` принимает несколько значений через запятую.
- `tenant-settings-ui`: требование «Форма конфигурации уведомлений» уточняется — форма не отправляет незаполненное поле URL и не может неявно затереть сохранённый вебхук.

## Impact

- **Бэкенд**:
  - `services/escalation/internal/schedclient/client.go`, `services/notification/internal/schedclient/client.go` — заголовок `X-Admin-Key`;
  - `services/escalation/internal/config`, `services/notification/internal/config` — новый параметр конфигурации (ключ);
  - `services/scheduling/internal/handler/handler.go` (`GetTenantNotificationConfig`, `PutTenantNotificationConfig`) — условное маскирование и семантика пустого URL;
  - `pkg/auth` — признак «запрос аутентифицирован сервисным ключом» в контексте;
  - `services/incident/internal/handler/handler.go`, `services/incident/internal/store/store.go` — разбор и `IN`-фильтр статусов.
- **Фронтенд**: `frontend/src/pages/TenantSettingsPage.tsx` — не отправлять пустой URL; `frontend/src/api/incidents.ts` — без изменений (формат `join(',')` становится валидным).
- **Деплой**: `deploy/k8s/*/configmap.yaml` / Secret — выдача `ADMIN_KEY` сервисам escalation, notification и scheduling (одинаковое значение).
- **Безопасность**: общий статический сервисный ключ — осознанный компромисс P0; переход на Keycloak client-credentials фиксируется как follow-up вне этого изменения.
- **Тесты**: интеграционные тесты schedclient (заголовок), handler scheduling (маскирование по типу аутентификации, PUT с пустым URL), store incident (мульти-статус).
