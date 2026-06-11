# Сверка спецификаций OpenSpec с фактическим кодом

Дата: 2026-06-11
Объём: все 12 спецификаций из `openspec/specs/` против кода `services/*`, `pkg/*`, `frontend/src/*`.

## Итоговая сводка

| Спецификация | Соответствие | Замечания |
|---|---|---|
| alert-ingestion | 🟡 в основном | TTL дедупликации по умолчанию не совпадает; source `prometheus` вместо `alertmanager` |
| incident-management | 🟡 в основном | пустой `tenant_slug` в `incident.created` (критично для нижестоящих сервисов); правила группировки не валидируют source |
| escalation-policies | 🟡 в основном | эндпоинт `/state` не возвращает историю и использует другое имя поля; зависит от пустого `tenant_slug` |
| notification-dispatch | 🟢 соответствует | мелкое замечание по логированию сбоя запроса конфигурации |
| oncall-scheduling | 🟢 соответствует | — |
| tenant-management | 🟡 в основном | параллельный немаскированный эндпоинт notification-config вне спецификации |
| frontend-shell | 🟢 соответствует | — |
| incident-dashboard-ui | 🟡 в основном | звук включён по умолчанию (спека: выключен); хоткей `/` не реализован; колонка «Источник» отсутствует; словарь severity расходится с бэкендом |
| escalation-policy-ui | 🟢 соответствует | — |
| oncall-schedule-ui | 🟡 в основном | тело ответа 409 не содержит дат конфликта, которых ожидает UI |
| tenant-settings-ui | 🟢 соответствует | — |
| user-profile-ui | 🟢 соответствует | — |

Активных изменений в `openspec/changes/` нет (только архив), поэтому сверка велась напрямую «спека ↔ код».

---

## Критичные расхождения

### 1. Пустой `tenant_slug` в событии `incident.created` (основной поток алертов)

**Спека:** `incident-management` — событие `incident.created` должно содержать `tenant_slug`; `escalation-policies` — `tenant_slug` используется в `escalation.triggered`; notification строит по нему ссылку и запрашивает конфигурацию тенанта.

**Код:** consumer incident-сервиса при создании инцидента из алерта не заполняет `TenantSlug` ([consumer.go:132-137](services/incident/internal/consumer/consumer.go#L132-L137)) — в БД пишется значение по умолчанию `''` ([000001_init.up.sql:6](services/incident/migrations/000001_init.up.sql#L6)), и событие уходит с пустым `tenant_slug`. HTTP-обработчик PATCH, напротив, корректно берёт slug из URL ([handler.go:191](services/incident/internal/handler/handler.go#L191)).

**Последствия по цепочке:**
- escalation при авто-назначении политики сохраняет пустой `TenantSlug` (fallback есть только в ручном пути привязки — [handler.go:222-224](services/escalation/internal/handler/handler.go#L222-L224));
- запрос дежурного уходит на `/api/schedules/v1//schedules/{id}/oncall` и завершается ошибкой → `escalation.triggered` публикуется с пустыми `oncall_user_id`/`oncall_username` (формально «наблюдаемо», но систематически, для каждого авто-созданного инцидента);
- notification не может получить per-tenant конфигурацию Mattermost (`GET /tenants//notification-config`) и строит битую ссылку `{base}//incidents?...`.

В системе `tenant_id` фактически совпадает со slug (Redis-индекс токенов хранит slug), так что данные доступны — их просто не прокидывают в `TenantSlug`. E2E-тесты эскалацию/уведомления не покрывают, поэтому дефект не ловится.

### 2. Маскирование вебхука обходится параллельным эндпоинтом

**Спека:** `tenant-management` — `mattermost_webhook_url` маскируется для пользовательских JWT; PUT с пустым URL не затирает сохранённый.

**Код:** канонические эндпоинты `/api/schedules/v1/tenants/{slug}/notification-config` реализованы точно по спеке ([handler.go:657-716](services/scheduling/internal/handler/handler.go#L657-L716)). Но существует второй, не описанный в спецификации набор `/api/schedules/v1/{tenant}/notification-config` (GET/PUT, доступен любому участнику тенанта), который:
- возвращает **немаскированный** URL любому участнику ([handler.go:438-449](services/scheduling/internal/handler/handler.go#L438-L449));
- при PUT с пустым URL **затирает** сохранённый вебхук ([handler.go:451-463](services/scheduling/internal/handler/handler.go#L451-L463)).

Фронтенд этим эндпоинтом не пользуется, но он нарушает требование маскирования и защиту от затирания. Рекомендация: удалить дублирующие маршруты или привести их к политике канонических.

---

## Существенные расхождения

### 3. TTL дедупликации: 4 часа в коде против 5 минут в спеке

Спека `alert-ingestion`: «настраиваемый TTL (по умолчанию 5 минут)». Код: `DEDUP_TTL_SECONDS` по умолчанию **4 часа** ([config.go:27](services/ingestion/internal/config/config.go#L27)). Поведение системы заметно отличается: повторный firing того же алерта будет подавляться часами. Нужно либо поправить спеку, либо дефолт.

> **Статус:** исправлено изменением `fix-substantial-spec-code-gaps` — спека выровнена по коду (дефолт зафиксирован как 4 часа, rationale — `repeat_interval` Alertmanager).

### 4. Source алертов Alertmanager: `prometheus` вместо `alertmanager`

Ingestion нормализует алерты Alertmanager с `source: "prometheus"` ([alertmanager.go:47](services/ingestion/internal/handler/alertmanager.go#L47), [alert.go](pkg/domain/alert.go)). При этом:
- спека `incident-management` и API группировки оперируют source `alertmanager` (`PUT /grouping-rules/alertmanager`, дефолты, список правил — [store.go:520](services/incident/internal/store/store.go#L520));
- правило, заданное администратором для `alertmanager`, **никогда не применится** к реальным алертам: consumer ищет правило по `source="prometheus"` ([consumer.go:118](services/incident/internal/consumer/consumer.go#L118)), а дефолты совпадают только потому, что `DefaultGroupingLabels` дублирует ветку `"alertmanager", "prometheus"` ([incident.go:84](services/incident/internal/domain/incident.go#L84));
- вебхук-токены создаются с `source: alertmanager` — в списке алертов инцидента и в токенах фигурируют разные имена одного источника.

> **Статус:** исправлено изменением `fix-substantial-spec-code-gaps` — `SourcePrometheus` → `SourceAlertmanager` (`alertmanager`), backfill-миграции historical `source`, алиас `prometheus → alertmanager` в consumer на переходный период.

### 5. Словарь severity: фронтенд ≠ бэкенд

Бэкенд использует `critical | high | warning | info` ([alert.go](pkg/domain/alert.go), `mapSeverity` в [normalize.go:33-44](services/ingestion/internal/handler/normalize.go#L33-L44)). Фронтенд объявляет и фильтрует `critical | high | medium | low` ([types.ts:4](frontend/src/api/types.ts#L4), селектор в [IncidentListPage.tsx:206-211](frontend/src/pages/IncidentListPage.tsx#L206-L211)). Итог: фильтры «Средний»/«Низкий» никогда ничего не находят, а инциденты с severity `warning`/`info` отображаются без бейджа (undefined-классы). Спеки значений severity для UI не фиксируют — расхождение между компонентами, его стоит закрыть в спеке и коде.

> **Статус:** исправлено изменением `fix-substantial-spec-code-gaps` — фронтенд приведён к словарю бэкенда `critical | high | warning | info`; спека `incident-dashboard-ui` фиксирует перечень.

### 6. Звуковые уведомления включены по умолчанию

Спека `incident-dashboard-ui`: переключатель «по умолчанию выключенный». Код: `localStorage.getItem('oncall.audioEnabled') !== 'false'` — при отсутствии ключа звук **включён** ([useAudioEnabled.ts:5](frontend/src/hooks/useAudioEnabled.ts#L5)). Остальное (Web Audio, отсутствие звука при скрытой вкладке, персистентность) соответствует.

> **Статус:** исправлено изменением `fix-substantial-spec-code-gaps` — инициализация заменена на `=== 'true'`, при отсутствии ключа звук выключен.

### 7. Ответ 409 при конфликте переопределений не содержит деталей

Спека `oncall-schedule-ui`: встроенная ошибка «с описанием конфликтующего окна». Бэкенд возвращает `409 {"error": "override window overlaps..."}` без дат ([handler.go:358-360](services/scheduling/internal/handler/handler.go#L358-L360)), а фронтенд ожидает `existing_start/existing_end/existing_user` ([schedules.ts:34-47](frontend/src/api/schedules.ts#L34-L47)) и отрендерит «Invalid Date — Invalid Date (undefined)» ([CreateOverrideModal.tsx:45-49](frontend/src/pages/CreateOverrideModal.tsx#L45-L49)). Сама inline-ошибка в модальном окне есть; не хватает контракта данных.

> **Статус:** исправлено изменением `fix-substantial-spec-code-gaps` — `CreateOverride` возвращает конфликтующее окно, handler сериализует `existing_start/existing_end/existing_user` (RFC3339); спека `oncall-scheduling` фиксирует формат тела 409.

---

## Мелкие расхождения

| # | Где | Спека | Код |
|---|---|---|---|
| 8 | escalation `/state` | ответ содержит `current_tier`, `next_escalation_at` и «полную историю уровней» | возвращается `EscalationState` c полем `escalate_at` (другое имя) и **без** истории; история — только отдельным эндпоинтом `/history` ([handler.go:256-267](services/escalation/internal/handler/handler.go#L256-L267), [policy.go:34](services/escalation/internal/domain/policy.go#L34)) |
| 9 | incident grouping-rules | поддерживаемые источники — `alertmanager`, `grafana` | `PUT /grouping-rules/{source}` принимает любой source без валидации (нет 422) ([handler.go:400-418](services/incident/internal/handler/handler.go#L400-L418)) |
| 10 | дашборд, колонки таблицы | «Критичность, Название, Статус, **Источник**, Создан, Подтверждён кем» | вместо «Источник» — колонка «Alertname» ([IncidentListPage.tsx:230](frontend/src/pages/IncidentListPage.tsx#L230)) |
| 11 | дашборд, хоткей `/` | переводит фокус в поле поиска/фильтра | обработчик пустой (`'/': () => {}`), поля поиска нет ([IncidentListPage.tsx:162](frontend/src/pages/IncidentListPage.tsx#L162)) |
| 12 | notification, сбой запроса конфигурации | «логирует структурированную ошибку с уровнем error и тенантом» | в `NotifyTriggered` ошибка `cache.Get` молча игнорируется (`cfg, _ :=`); error-лог появляется только если у контакта включён Mattermost ([notifier.go:116](services/notification/internal/notifier/notifier.go#L116)). Email-фоллбэк и запись `failed` при маскированном URL реализованы корректно |
| 13 | мобильный список смен | «список предстоящих смен на ближайшие 7 дней» | список строится из окна текущего месяца; если 7 дней пересекают границу месяца, хвост смен следующего месяца не показывается ([SchedulesPage.tsx:225-235](frontend/src/pages/SchedulesPage.tsx#L225-L235)) |
| 14 | retry публикации | «до 3 повторных попыток с экспоненциальной задержкой» | реализовано 3 попытки всего (2 ретрая) c backoff 1s/2s ([amqp.go, Publish](pkg/amqp/amqp.go)); трактовка «попытки vs ретраи» расходится |

> **Статус (пункты 8–14):** исправлено изменением `fix-minor-spec-code-gaps`. Направление по пунктам:
>
> - **8** (спека→код): `/state` зафиксирован как `current_tier | status | escalate_at` без истории (история — отдельный `/history`).
> - **9** (код→спека): `PUT/DELETE /grouping-rules/{source}` отклоняют source вне `alertmanager | grafana` с HTTP 422.
> - **10** (спека→код): колонка таблицы зафиксирована как «Alertname»; источник алертов — на вкладке «Алерты».
> - **11**: хоткей `/` удалён из спеки и мёртвая пустая привязка — из кода.
> - **12** (код→спека): ошибка `cache.Get` в `NotifyTriggered` логируется с уровнем `error` (`tenant_slug`, `incident_id`), доставка продолжается с фоллбэками.
> - **13** (код→спека): мобильный список «Ближайшие 7 дней» строится по отдельному окну `[сегодня, сегодня+7д]`, не теряя смены на границе месяца.
> - **14** (спека→код): формулировка ретраев приведена к «до 3 попыток (первая + до 2 повторных)».

---

## Что сверено и совпадает (выборочно)

**alert-ingestion:** заголовок `X-Webhook-Token` + SHA-256 + Redis `oncall:tokens:{hash}` без чтения чужих таблиц; эндпоинты `/api/ingest/v1/webhook/{alertmanager,grafana}`; маппинг состояний Grafana (`ok|paused → resolved`); каноническая схема алерта (все поля); fingerprint по отсортированным лейблам + source + tenant; SETNX-дедупликация, resolved обходит дедуп и удаляет ключ; синхронная публикация в exchange `alerts` / `alert.received`; при сбое публикации — удаление dedup-ключа, error-лог и HTTP 503.

**incident-management:** создание инцидента из firing-алерта + событие `incident.created` со всеми полями (кроме п.1); severity/заголовок от первого алерта и не пересчитываются; частичный resolve (статус `open`, пока есть firing-алерты, `MaybeResolve`); resolved без открытого инцидента — ack без ошибки; группировка по group_key из правил per-source с дефолтами; лейблы копируются только при создании, не мёржатся при догруппировке; PUT labels мёржит; комментарии (201, автор из `sub`, сортировка по времени); история append-only (нет API изменения/удаления); статусы `open→acknowledged→resolved→open` через state machine с 422; список с multi-status фильтром через запятую, 400 на недопустимое значение, курсорная пагинация; изоляция тенанта (404 для чужого инцидента, `RequireTenantMember` 403).

**escalation-policies:** CRUD политик без PUT; 422 на пустые tiers и неположительный таймаут; default-policy (PUT с проверкой принадлежности → 422, GET 404, DELETE); авто-назначение по `incident.created` с немедленным триггером tier 1; обогащение данных инцидента из события (авто) и из incident-сервиса с `X-Admin-Key` + warn-фоллбэк (ручная привязка); запрос дежурного с `X-Admin-Key`; error-лог при сбое резолва дежурного; остановка по `acknowledged` и `resolved` с записью причины в историю; `exhausted` + событие `escalation.exhausted`; ручная эскалация; история эскалации с типами событий, tier и дежурным.

**notification-dispatch:** потребление `escalation.triggered`/`exhausted` без синхронных вызовов за on-call-данными; фоллбэк-контент при отсутствии полей инцидента (email-тема и Mattermost-текст); email со всеми требуемыми полями и ссылкой `{base}/{slug}/incidents?incident={id}`; warn при отсутствии `FRONTEND_BASE_URL`; 3 попытки с backoff для SMTP и Mattermost; rate-limit 5/600с через Redis-Lua token bucket со статусом `rate_limited` и warn-логом; журнал `delivered|failed|rate_limited`; контакты `(user_id, tenant_id)` + `enabled_channels`; per-tenant Mattermost-конфиг через scheduling с `X-Admin-Key`; защита от маскированного URL (`webhookURLUsable`) с `failed`-записью.

**oncall-scheduling / tenant-management:** CRUD расписаний с 422 на отсутствующие поля; oncall с `?at`; переопределения с 409 на пересечение и приоритетом над ротацией; `/shifts?from&to` с `is_override`; удаление расписания каскадно удаляет переопределения (FK `ON DELETE CASCADE`); тенанты (slug unique → 409, PATCH только name, POST без тенантной роли); удаление тенанта не трогает связанные данные (FK на тенанта нет — документированное ограничение соблюдено); участники через Keycloak Admin API с ролями по подгруппе `admins`; токены: генерация сервером, однократный показ, SHA-256 в БД + Redis HSET/HDEL, 422 на source вне `alertmanager|grafana`, несколько активных токенов на источник; маскирование URL по способу аутентификации с безопасным умолчанием; PUT c пустым URL сохраняет текущий (канонический эндпоинт).

**frontend-shell:** PKCE через `oidc-client-ts`, токен в `sessionStorage`, silent renew через iframe (порог 120с); баннер с 30-секундным отсчётом только при близком истечении или ≥3 сбоях подряд, сброс при успешном renew; экраны ошибок входа и `/callback` без циклов редиректов; `parseGroups` согласован с `pkg/auth` (членство по префиксу, admin по `/{tenant}/admins`); диагностика неверного маппера (непустой groups + пустая карта); TenantGuard/AdminGuard с 403; `/select-team`, прямой редирект при одной команде; сайдбар с 4 разделами (настройки скрыты для member), шапка с username/темой/звуком; тёмная тема по умолчанию, применение из `localStorage` до первой отрисовки (inline-скрипт в `index.html`).

**incident-dashboard-ui:** поллинг 12с; multi-status фильтр в URL и через запятую в API; панель деталей с вкладками Алерты/История/Комментарии и `?incident=<id>`; события эскалации в таймлайн не подмешиваются; оптимистичные Подтвердить/Закрыть с откатом и toast; состояния кнопок по статусу; хоткеи A/R/J/K/Escape с блокировкой при фокусе в полях ввода; звук только при видимой вкладке.

**escalation-policy-ui:** список с бейджем «По умолчанию» и подтверждением удаления; вертикальный степпер с выбором расписания, таймаутом, переупорядочиванием; сохранение заменой POST → перенос default → DELETE старой (ровно как в спеке); валидация незаполненного шага.

**tenant-settings-ui / user-profile-ui:** одноразовый показ токена с обязательным чекбоксом и очисткой из состояния; маскированный URL не предзаполняется и показывается справочно; PUT без поля URL, если оно не вводилось; readonly-список участников с баннером о Keycloak; селектор тенанта в профиле, 404 → пустая форма, guard включения канала без контактов, немедленный PUT при переключении.

---

## Рекомендации (по убыванию приоритета)

1. **Заполнить `tenant_slug` в incident-consumer** (или передавать `tenant_id` как slug в события) — чинит цепочку эскалация → уведомления для основного потока. Добавить e2e-покрытие escalation/notification.
2. **Удалить или защитить** дублирующие эндпоинты `/api/schedules/v1/{tenant}/notification-config`.
3. Согласовать **словарь severity** фронтенда и бэкенда (`warning/info` vs `medium/low`).
4. Решить судьбу **source `prometheus` vs `alertmanager`** (унифицировать имя или маппить при поиске правил группировки) и добавить 422-валидацию source в grouping-rules.
5. Привести **TTL дедупликации** (спека 5 минут ↔ код 4 часа) к одному значению.
6. Вернуть дефолт **звука = выключен**; добавить детали конфликта в тело 409 переопределений; реализовать или убрать из спеки хоткей `/` и колонку «Источник»; выровнять контракт `/state` (имя поля, история).
