# Задачи

## 1. Scaffolding проекта

- [x] 1.1 Инициализировать проект Vite + React + TypeScript в директории `frontend/` через `npm create vite@latest`
- [x] 1.2 Установить и настроить Tailwind CSS с `darkMode: 'class'` и Shadcn/ui CLI
- [x] 1.3 Установить `react-router-dom` v6, `@tanstack/react-query` v5, `oidc-client-ts`, `date-fns`, `date-fns-tz`
- [x] 1.4 Настроить path aliases (`@/` → `src/`) в `vite.config.ts` и `tsconfig.json`
- [x] 1.5 Настроить ESLint + Prettier с общей конфигурацией (TypeScript, React, порядок импортов)
- [x] 1.6 Добавить Dockerfile для раздачи статики через nginx и манифест Kubernetes Deployment в `./deploy/k8s/frontend/`

## 2. Оболочка аутентификации

- [x] 2.1 Создать `src/auth/oidcConfig.ts` — настроить `UserManager` с PKCE, `sessionStorage` и параметрами тихого обновления (окно уведомления 120 сек)
- [x] 2.2 Реализовать контекст `<AuthProvider>`, оборачивающий `UserManager`, с экспортом `user`, `signIn`, `signOut` и хука `usePermissions()`
- [x] 2.3 Реализовать хук `usePermissions()`, декодирующий claim `groups` из `id_token` и возвращающий `{ [tenantSlug]: 'member' | 'admin' }`
- [x] 2.4 Создать обработчик маршрута `/callback`, завершающий PKCE-обмен и перенаправляющий на исходный URL назначения
- [x] 2.5 Реализовать обёртку `<RequireAuth>`, перенаправляющую неаутентифицированных пользователей на вход в Keycloak с сохранением `state`
- [x] 2.6 Реализовать баннер истечения сессии, появляющийся за 120 сек до истечения токена и перенаправляющий на вход через 30 сек

## 3. Роутинг и макет

- [x] 3.1 Описать полное дерево маршрутов в `src/routes.tsx` (`/`, `/select-team`, `/[tenant]/*`, `/profile`, `/callback`, `/403`)
- [x] 3.2 Реализовать `<TenantGuard>` — читает slug тенанта из URL-параметра, проверяет `usePermissions()`, рендерит 403 если не участник
- [x] 3.3 Реализовать `<AdminGuard>` — оборачивает `/[tenant]/settings`, проверяет роль `admin`, рендерит 403 если не администратор
- [x] 3.4 Собрать компонент `GlobalLayout`: боковая навигация (Инциденты, Расписания, Эскалации, Настройки), шапка с названием тенанта, логином пользователя, переключателем темы, переключателем звука
- [x] 3.5 Собрать страницу `/select-team` со списком доступных тенантов из `usePermissions()` с навигацией по клику; автоматический редирект на `/[tenant]/incidents` при единственном тенанте
- [x] 3.6 Сохранять предпочтение тёмной темы в `localStorage` (`oncall.colorScheme`); применять до первой отрисовки через встроенный скрипт в `index.html`
- [x] 3.7 Реализовать хук `useKeyMap` для глобальной диспетчеризации горячих клавиш (пропускает события, когда input/textarea в фокусе)

## 4. Слой API-клиента

- [x] 4.1 Создать `src/api/client.ts` — обёртка над axios, подставляющая `Authorization: Bearer <token>` из `sessionStorage` в каждый запрос
- [x] 4.2 Написать TypeScript-типы для всех форм ответов бэкенда (Incident, Alert, Comment, HistoryEntry, Schedule, Override, EscalationPolicy, WebhookToken, NotificationConfig, UserContacts)
- [x] 4.3 Создать модуль query-хуков `src/api/incidents.ts`: `useIncidents(tenant, filters)`, `useIncident(tenant, id)`, `useIncidentAlerts`, `useIncidentHistory`, `useIncidentComments`
- [x] 4.4 Создать mutation-хуки: `useAcknowledgeIncident`, `useResolveIncident`, `usePostComment` — с оптимистичными обновлениями и откатом
- [x] 4.5 Создать query-хуки `src/api/schedules.ts`: `useSchedules(tenant)`, `useOnCallNow(tenant)`, `useScheduleWindow(tenant, scheduleId, from, to)`
- [x] 4.6 Создать mutation-хуки: `useCreateOverride`, `useDeleteOverride` с извлечением ошибки 409-конфликта
- [x] 4.7 Создать query/mutation-хуки `src/api/escalations.ts`: `useEscalationPolicies`, `useCreatePolicy`, `useUpdatePolicy`, `useDeletePolicy`, `useSetDefaultPolicy`
- [x] 4.8 Создать query/mutation-хуки `src/api/tenantSettings.ts`: `useWebhookTokens`, `useCreateToken`, `useRevokeToken`, `useNotificationConfig`, `useSaveNotificationConfig`, `useMembers`
- [x] 4.9 Создать query/mutation-хуки `src/api/profile.ts`: `useUserContacts`, `useSaveUserContacts`

## 5. Дашборд инцидентов

- [x] 5.1 Собрать `IncidentListPage` с таблицей Shadcn/ui; колонки: бейдж критичности, Название, чип статуса, Источник, Создан, Подтверждён кем
- [x] 5.2 Добавить панель фильтров с мультиселектом статуса (open/acknowledged/resolved), селектором критичности и текстовым полем источника; синхронизация фильтров с URL search params
- [x] 5.3 Настроить `refetchInterval: 12_000` в `useIncidents` и реализовать аудиотриггер нового инцидента (сравнение наборов ID между запросами)
- [x] 5.4 Реализовать хук `useAudioNotification` — синтезатор сигнала через Web Audio API; читает/записывает `oncall.audioEnabled` в `localStorage`; пропускает при `document.hidden`
- [x] 5.5 Собрать компонент drawer `IncidentDetailPanel` с `Tabs` (Алерты, История, Комментарии)
- [x] 5.6 Собрать вкладку «Алерты»: список алертов с бейджами лейблов «ключ: значение» и индикатором firing/resolved
- [x] 5.7 Собрать вкладку «История»: компонент вертикального таймлайна, рендерящий изменения статуса, события ACK, триггеры эскалации и комментарии в хронологическом порядке
- [x] 5.8 Собрать вкладку «Комментарии»: список сообщений в стиле чата + textarea + кнопка отправки; привязать к mutation `usePostComment`
- [x] 5.9 Добавить кнопки «Подтвердить» и «Закрыть» с состоянием загрузки, логикой блокировки по статусу инцидента и привязкой оптимистичных обновлений
- [x] 5.10 Синхронизировать ID выбранного инцидента с URL-параметром `?incident=<id>`; восстанавливать состояние drawer при загрузке страницы из параметра
- [x] 5.11 Подключить горячие клавиши через `useKeyMap`: `A`=подтвердить, `R`=закрыть, `/`=фокус фильтра, `J`/`K`=навигация по списку, `Esc`=закрыть drawer

## 6. UI расписаний дежурств

- [x] 6.1 Собрать скелет `SchedulesPage` с виджетом «Кто дежурит сейчас» вверху (поллинг каждые 60 сек через `useOnCallNow`)
- [x] 6.2 Реализовать компонент Gantt-сетки: рендер шапки месяца с колонками дней, по одной строке на ротацию, полосы смен как позиционированные `div`-ы, вычисленные из данных окна
- [x] 6.3 Реализовать клиентское вычисление окна смены через `date-fns-tz` (конвертация UTC-границ смен в локальный часовой пояс, вычисление процентных значений left/width для полос)
- [x] 6.4 Добавить элементы навигации по месяцам (пред/след); повторно запрашивать данные окна расписания при навигации на новый диапазон месяца
- [x] 6.5 Реализовать мобильный резервный вид: скрыть Gantt на брейкпоинте `<640px`; рендерить список предстоящих смен на 7 дней
- [x] 6.6 Рендерить полосы переопределений на Gantt с визуально отличимым паттерном (штриховой фон или другой оттенок)
- [x] 6.7 Собрать `CreateOverrideModal`: пикеры диапазона дат (начало/конец), селектор участника из `useMembers`, отправка через `useCreateOverride`
- [x] 6.8 Обработать ошибку 409-конфликта в `CreateOverrideModal`: показывать встроенную ошибку с деталями конфликтующего окна, оставлять модальное окно открытым

## 7. UI политик эскалации

- [x] 7.1 Собрать `EscalationPoliciesPage` со списком политик: название, количество шагов, бейдж «По умолчанию» и кнопки действий (редактировать, удалить, сделать по умолчанию)
- [x] 7.2 Реализовать диалог подтверждения удаления; привязать к mutation `useDeletePolicy`
- [x] 7.3 Собрать `PolicyEditorPage` (создание/редактирование): вертикальный степпер, рендерящий каждый шаг политики как карточку
- [x] 7.4 Карточка шага: выпадающий список расписаний (из `useSchedules`), поле таймаута (целое число, мин. 1), кнопка удаления, стрелки вверх/вниз
- [x] 7.5 Добавить валидацию шагов: встроенная ошибка при не выбранном расписании; блокировка кнопки сохранения до заполнения всех шагов
- [x] 7.6 Привязать кнопку «Сохранить» к `useUpdatePolicy` или `useCreatePolicy` в зависимости от режима создания/редактирования; показывать toast об успехе
- [x] 7.7 Привязать действие «Сделать по умолчанию» к `useSetDefaultPolicy`; обновлять список для актуализации бейджа «По умолчанию»

<!-- ═══════════════════════════════════════════════════════════════════════════
  БУДУЩАЯ ФИЧА: Управление эскалацией инцидента
  Эндпоинты бекенда реализованы, фронтенд не реализован.
  Запланировать отдельным изменением (openspec new change incident-escalation-ui).

  Бекенд API (порт 8083):
    POST   /api/escalations/v1/{tenant}/incidents/{incidentId}/policy
           Тело: { policy_id: string, tenant_slug: string }
           Ответ: EscalationState — активирует политику для инцидента
    GET    /api/escalations/v1/{tenant}/incidents/{incidentId}/state
           Ответ: EscalationState { id, incident_id, tenant_id, policy_id,
                  current_tier, status, escalate_at, created_at, updated_at }
    POST   /api/escalations/v1/{tenant}/incidents/{incidentId}/escalate
           Тело: пустое — вручную продвигает на следующий уровень
           Ответ: обновлённый EscalationState
    GET    /api/escalations/v1/{tenant}/incidents/{incidentId}/history
           Ответ: EscalationHistory[] { id, incident_id, event_type,
                  tier, oncall_user_id, oncall_username, created_at }

  Что нужно сделать на фронте:
  — Хуки в src/api/escalations.ts:
      useEscalationState(tenant, incidentId)  → GET /state, поллинг 30s
      useAttachPolicy(tenant, incidentId)     → POST /policy
      useManualEscalate(tenant, incidentId)   → POST /escalate
      useEscalationHistory(tenant, incidentId)→ GET /history
  — Типы в src/api/types.ts:
      EscalationState, EscalationHistory (поля перечислены выше)
  — В IncidentDetailPanel (src/pages/IncidentDetailPanel.tsx):
      Новый раздел / вкладка «Эскалация»:
        · Виджет текущего состояния: текущий уровень (tier), статус,
          время следующей автоэскалации (escalate_at, обратный отсчёт)
        · Кнопка «Назначить политику» → модалка с select из useEscalationPolicies
          (POST /policy), показывать только если policy не назначена
        · Кнопка «Эскалировать сейчас» → POST /escalate с confirm-диалогом,
          активна только при status=active и current_tier < max_tier
        · Хронология: список событий из GET /history (event_type: triggered /
          tier_advanced / acknowledged / resolved / exhausted), аналог вкладки «История»
  — Статус escalation.status отображать в карточке инцидента в списке (опционально)
════════════════════════════════════════════════════════════════════════════ -->

## 8. UI настроек тенанта

- [ ] 8.1 Собрать `TenantSettingsPage` с тремя секциями: Webhook-токены, Конфигурация уведомлений, Участники
- [ ] 8.2 Секция Webhook-токенов: список из `useWebhookTokens`, кнопка «Отозвать» на каждой строке с диалогом подтверждения (привязать к `useRevokeToken`)
- [ ] 8.3 Собрать `GenerateTokenModal`: поле ввода метки источника + отправка; при успехе показывать `OneTimeTokenRevealModal`
- [ ] 8.4 Собрать `OneTimeTokenRevealModal`: копируемый `<pre>` с текстом токена, обязательный чекбокс «Я скопировал этот токен», кнопка «Закрыть» активна только при отмеченном чекбоксе; очищать токен из состояния при закрытии
- [ ] 8.5 Секция конфигурации уведомлений: форма с `mattermost_webhook_url` (валидация URL), `mattermost_channel`, `smtp_from` (валидация email); сохранение через `useSaveNotificationConfig`
- [ ] 8.6 Секция участников: список только для чтения из `useMembers`; статический информационный баннер об управлении составом команды через Keycloak

## 9. UI профиля пользователя

- [ ] 9.1 Собрать `ProfilePage` по адресу `/profile` с формой контактов: поле имени пользователя в Mattermost, поле email (валидация формата)
- [ ] 9.2 Привязать форму к `useUserContacts` (загрузка) и `useSaveUserContacts` (сохранение); показывать toast об успехе
- [ ] 9.3 Добавить переключатели каналов уведомлений (email, mattermost), отражающие `enabled_channels`; переключение немедленно вызывает `useSaveUserContacts` с обновлённым массивом
- [ ] 9.4 Реализовать гард: при включении переключателя Mattermost без указанного имени пользователя — показывать встроенное предупреждение и возвращать переключатель в исходное положение

## 10. Качество и сборка

- [ ] 10.1 Написать юнит-тесты для парсинга claim `groups` в `usePermissions()` (несколько тенантов, отсутствующий groups, роль admin)
- [ ] 10.2 Написать юнит-тесты для вычисления окна смены в Gantt (включая граничный случай с переходом DST)
- [ ] 10.3 Написать юнит-тесты для триггера аудиосигнала нового инцидента (обнаружение новых ID со статусом `open`)
- [ ] 10.4 Добавить `vite build` в CI pipeline; завершать с ошибкой при наличии ошибок компиляции TypeScript
- [ ] 10.5 Настроить `nginx.conf` для раздачи `index.html` на все маршруты (SPA fallback) и установки корректных MIME-типов и заголовков кэширования для статических ресурсов
- [ ] 10.6 Обновить корневой `helmfile.yaml`, добавив Deployment и Service фронтенда; настроить Ingress для раздачи `/` из контейнера фронтенда
