# Задачи

## 1. Репозиторий и инфраструктура

- [x] 1.1 Инициализировать Go-монорепо с `go.work`; создать скелеты модулей для `ingestion`, `incident`, `scheduling`, `escalation`, `notification`
- [x] 1.2 Добавить общие пакеты `pkg/`: обёртка AMQP-клиента (RabbitMQ, библиотека `amqp091-go`), хелпер пула соединений PostgreSQL, хелпер Redis-клиента, структурированный логгер (zap/slog), `pkg/auth` — JWT middleware (JWKS-валидация, парсинг `sub` и `groups` claims, bypass по `X-Admin-Key`)
- [x] 1.3 Написать `docker-compose.yaml` для локального k3s: PostgreSQL, Redis, RabbitMQ, Keycloak; в prod все внешние сервисы, подключаются через Secret
- [x] 1.4 Создать шаблоны Kubernetes-манифестов (Deployment, Service, ConfigMap, Secret) для всех пяти сервисов
- [x] 1.5 Настроить Ingress с маршрутами `/api/ingestion/*`, `/api/incidents/*`, `/api/schedules/*`, `/api/escalations/*`, `/api/notifications/*`
- [x] 1.6 Настроить `golang-migrate`; создать общий хелпер запуска миграций, используемый каждым сервисом при старте

## 2. Общие доменные типы

- [x] 2.1 Определить каноническую структуру `Alert` в `pkg/domain/alert.go` (fingerprint, source, severity, title, description, labels, status, fired_at, received_at)
- [x] 2.2 Определить константы AMQP-топологии в `pkg/amqp/topology.go`: exchange `alerts` → очередь `alerts.incident`; exchange `incidents` → очередь `incidents.escalation`; exchange `escalations` → очередь `escalations.notification`
- [x] 2.3 Определить JSON-envelope и соглашение о версионировании для всех AMQP-сообщений

## 3. Сервис приёма алертов (ingestion)

- [ ] 3.1 Создать PostgreSQL-схему `ingestion`; миграция: таблица `raw_alerts` (id, tenant_id, fingerprint, source, payload, received_at, deduplicated)
- [ ] 3.2 Реализовать обработчик вебхука Prometheus Alertmanager: разбор payload v4, нормализация в канонический Alert
- [ ] 3.3 Реализовать обработчик вебхука Grafana: разбор payload, нормализация в канонический Alert, обработка `state: ok` как resolved
- [ ] 3.4 Реализовать middleware идентификации тенанта: вычислить SHA-256 хэш `X-Webhook-Token`, выполнить `HGET oncall:tokens:{hash}` в Redis → получить `tenant_id`; возвращать HTTP 401 при отсутствии совпадения; scheduling обновляет этот Redis-индекс при создании/отзыве токенов
- [ ] 3.5 Реализовать дедупликацию через Redis: SHA-256 fingerprint по отсортированным лейблам + source + tenant_id; SETNX с настраиваемым TTL; удаление ключа для resolved-алерта
- [ ] 3.6 Реализовать AMQP-публикатор на exchange `alerts` (routing key `alert.received`) с повторными попытками и backoff при ошибках
- [ ] 3.7 Написать юнит-тесты для логики нормализации (все три источника), дедупликации и middleware токена
- [ ] 3.8 Написать интеграционный тест: POST на каждый эндпоинт с валидным токеном → проверить публикацию сообщения в RabbitMQ с корректным tenant_id

## 4. Сервис инцидентов (incident)

- [ ] 4.1 Создать PostgreSQL-схему `incident`; миграции: таблицы `incidents`, `incident_alerts` (alert_id, incident_id, status: firing|resolved), `incident_labels`, `incident_comments`, `incident_history`, `incident_grouping_rules` (tenant_id, source, grouping_labels[])
- [ ] 4.2 Реализовать AMQP-консьюмер очереди `alerts.incident`; для firing-алерта: создать инцидент или привязать к существующему по группирующим лейблам; для resolved-алерта: пометить алерт как `resolved` в `incident_alerts`, закрыть инцидент если все алерты resolved
- [ ] 4.11 Реализовать REST API группирующих лейблов (`/api/incidents/v1/{tenant}/grouping-rules`): GET (все три источника с флагом `is_default`), PUT `/{source}` (задать лейблы), DELETE `/{source}` (сброс к умолчанию); только для admin тенанта
- [ ] 4.3 Реализовать автомат состояний жизненного цикла инцидента: open → acknowledged → resolved; поддержка повторного открытия
- [ ] 4.4 Реализовать CRUD лейблов: объединение карт лейблов, сохранение в `incident_labels`
- [ ] 4.5 Реализовать CRUD комментариев: эндпоинты добавления, списка (по возрастанию) и удаления
- [ ] 4.6 Реализовать запись истории только на добавление: фиксировать каждое изменение статуса, мутацию лейбла, событие комментария
- [ ] 4.7 Реализовать REST API: все маршруты по префиксу `/api/incidents/v1/{tenant}/incidents`; GET список (фильтры: status, severity, label, from_time, to_time + пагинация), GET по ID, PATCH статус, POST привязка алертов, PUT лейблы, POST/GET/DELETE комментарии, GET история; автор комментария — `sub` из JWT
- [ ] 4.8 Публиковать AMQP-события `incident.created` и `incident.updated` на exchange `incidents` при изменениях состояния; payload включает `incident_id`, `tenant_id`, `tenant_slug` (нужен escalation service для формирования URL к scheduling)
- [ ] 4.9 Написать юнит-тесты для автомата состояний и логики объединения лейблов
- [ ] 4.10 Написать интеграционные тесты для AMQP-консьюмера и эндпоинтов REST API

## 5. Сервис расписаний (scheduling)

- [ ] 5.1 Создать PostgreSQL-схему `scheduling`; миграции: таблицы `tenants`, `users` (Keycloak-кэш: sub, preferred_username, name, email, last_seen_at), `tenant_webhook_tokens`, `tenant_notification_config`, `schedules`, `schedule_overrides`
- [ ] 5.2 Реализовать REST API управления расписаниями (`/api/schedules/v1/{tenant}/schedules`); валидация обязательных полей; `tenant_id` извлекается из slug в URL
- [ ] 5.3 Реализовать движок вычисления ротации: для заданного расписания и временной метки вернуть дежурного пользователя (с учётом shift_duration, start_date, timezone)
- [ ] 5.4 Реализовать хранение и поиск переопределений: проверять окна переопределений перед возвратом результата ротации
- [ ] 5.5 Реализовать эндпоинт GET `/{tenant}/schedules/{id}/oncall?at=<время>` с использованием движка вычисления
- [ ] 5.6 Реализовать CRUD переопределений расписания с валидацией пересечений (HTTP 409 при конфликте)
- [ ] 5.7 Реализовать эндпоинт списка смен (вычисляемый, не хранимый): генерировать список смен для запрошенного временного окна
- [ ] 5.8 Написать юнит-тесты для движка вычисления ротации (граничные случаи DST, приоритет переопределений)
- [ ] 5.9 Написать интеграционные тесты для эндпоинтов расписаний и переопределений

## 6. Сервис эскалаций (escalation)

- [ ] 6.1 Создать PostgreSQL-схему `escalation`; миграции: таблицы `policies`, `policy_tiers`, `incident_escalation_states`, `escalation_history`, `tenant_escalation_config` (tenant_id PK, default_policy_id FK → policies.id)
- [ ] 6.2 Реализовать REST API управления политиками эскалации (`/api/escalations/v1/{tenant}/policies`) и API политики по умолчанию: PUT/GET/DELETE `/api/escalations/v1/{tenant}/default-policy`; PUT валидирует, что policy_id принадлежит тенанту
- [ ] 6.3 Реализовать POST `/api/escalations/v1/{tenant}/incidents/{id}/policy` для привязки политики и запуска отслеживания с 1-го уровня
- [ ] 6.4 Реализовать монитор состояния эскалации: опрашивать `incident_escalation_states` каждые 30 сек на наличие уровней с истёкшим `escalate_at`; переходить на следующий уровень или устанавливать `exhausted`
- [ ] 6.5 Потреблять события из очереди `incidents.escalation`: `incident.created` → искать `default_escalation_policy_id` в `tenant_escalation_config`, при наличии авто-назначать политику; `incident.updated` → останавливать эскалацию при переходе инцидента в `acknowledged` или `resolved`
- [ ] 6.6 При переходе уровня: запросить scheduling (`GET /schedules/{notify_schedule_id}/oncall`) за текущим дежурным; опубликовать обогащённое AMQP-событие `escalation.triggered` на exchange `escalations` с полями `incident_id`, `tenant_id`, `tier`, `oncall_user_id`, `oncall_username`; при исчерпании — `escalation.exhausted`
- [ ] 6.7 Реализовать эндпоинт GET `/api/escalations/v1/{tenant}/incidents/{id}/state`
- [ ] 6.8 Реализовать POST `/api/escalations/v1/{tenant}/incidents/{id}/escalate` (ручной переход)
- [ ] 6.9 Написать юнит-тесты для логики перехода уровней и обработки исчерпания
- [ ] 6.10 Написать интеграционные тесты: (a) авто-назначение: задать default-policy тенанту → опубликовать `incident.created` → проверить запись в `incident_escalation_states`; (b) таймаут: создать инцидент с политикой → смоделировать истечение таймаута → проверить переход уровня и AMQP-событие `escalation.triggered`

## 7. Сервис уведомлений (notification)

- [ ] 7.1 Создать PostgreSQL-схему `notification`; миграции: таблицы `user_contacts`, `notification_log`
- [ ] 7.2 Реализовать REST API контактной конфигурации (`/api/notifications/v1/{tenant}/contacts/{userId}`)
- [ ] 7.3 Реализовать AMQP-консьюмер очереди `escalations.notification` (события `escalation.triggered`, `escalation.exhausted`); `oncall_user_id` и `oncall_username` берутся из payload события — HTTP-вызов в scheduling за on-call данными не нужен; для tenant notification config реализовать in-process LRU-кэш с TTL 5 минут (промах → HTTP в scheduling)
- [ ] 7.4 Реализовать Email-диспетчер: формировать сообщение (ID инцидента, заголовок, severity, статус, временная метка), отправлять через SMTP с повторными попытками и backoff
- [ ] 7.5 Реализовать Mattermost-диспетчер: формировать сообщение в канал тенанта (из `tenant_notification_config`), включать @mention `mattermost_username` дежурного если задан; POST на webhook URL с повторными попытками и backoff; DM не используется
- [ ] 7.6 Реализовать Redis token-bucket rate-limiter на контакт (настраиваемый максимум и окно); при превышении — отбросить уведомление, записать `rate_limited` в `notification_log`, вывести warning в лог; без очереди на повтор
- [ ] 7.7 Реализовать запись журнала доставки: фиксировать `delivered` / `failed` с деталями ошибок в `notification_log`
- [ ] 7.8 Соблюдать `enabled_channels` для каждого пользователя: пропускать отключённые каналы
- [ ] 7.9 Написать юнит-тесты для rate-limiter и логики диспетчеризации каналов
- [ ] 7.10 Написать интеграционные тесты для потока AMQP-консьюмер → диспетчеризация → журнал

## 8. Управление тенантами

- [ ] 8.1 Реализовать CRUD-эндпоинты тенантов (`/api/schedules/v1/tenants`): создание, получение, обновление имени, удаление; валидация уникальности slug
- [ ] 8.2 Реализовать GET `/api/schedules/v1/tenants/{slug}/members`: вызвать Keycloak Admin API (client credentials) для группы `{slug}` и подгруппы `{slug}/admins`; вернуть объединённый список `(user_id, preferred_username, role)`; мутации участников — только через Keycloak UI, платформа не предоставляет POST/DELETE
- [ ] 8.3 Реализовать управление вебхук-токенами (`/api/schedules/v1/tenants/{slug}/webhook-tokens`): генерация (показать один раз), сохранить SHA-256 хэш в БД и `HSET oncall:tokens:{hash} tenant_id` в Redis; отзыв — DELETE из БД + `HDEL` из Redis; поддержка нескольких токенов на источник
- [ ] 8.4 Реализовать CRUD конфигурации уведомлений тенанта (`/api/schedules/v1/tenants/{slug}/notification-config`): mattermost_webhook_url, mattermost_channel, smtp_from
- [ ] 8.5 Реализовать stateless `pkg/auth` middleware: валидировать Bearer JWT через JWKS (env `KEYCLOAK_JWKS_URL`), извлекать `sub`, `preferred_username`, `name`, `email`, `groups` из claims и класть в Go context; никакого IO из middleware; возвращать HTTP 401 при отсутствии/невалидном токене; подключить во все 5 сервисов. Tenant-проверку (наличие slug в `groups`, роль `admins`) реализовать как отдельный handler-level middleware поверх `pkg/auth` в каждом сервисе. `X-Admin-Key` обходит все проверки. Upsert в таблицу `users` выполнять только в scheduling service (не в middleware)
- [ ] 8.6 Добавить `tenant_id NOT NULL` во все доменные миграции (incidents, schedules, escalation_policies, notification_log); добавить индексы по `tenant_id` на все таблицы
- [ ] 8.7 Реализовать per-tenant конфигурацию уведомлений в сервисе notification: брать mattermost_webhook_url из tenant_notification_config через HTTP-запрос к scheduling
- [ ] 8.8 Написать интеграционные тесты: изоляция — пользователь тенанта A не видит данные тенанта B

## 9. Сквозное тестирование и операционная готовность

- [ ] 9.1 Написать сквозной тест: получить JWT через client credentials Keycloak test-realm → создать тенант (X-Admin-Key) → настроить вебхук-токен → отправить вебхук Prometheus → проверить инцидент создан в контексте тенанта → политика срабатывает → уведомление идёт на Mattermost-канал тенанта
- [ ] 9.2 Добавить эндпоинты `/healthz` и `/readyz` во все пять сервисов
- [ ] 9.3 Добавить эндпоинты Prometheus метрик (`/metrics`) во все сервисы: количество запросов, процент ошибок, глубину очередей RabbitMQ, hit rate дедупликации
- [ ] 9.4 Написать Helmfile / скрипт настройки k3s с README, описывающим процедуру запуска локальной разработки
- [ ] 9.5 Задокументировать переменные окружения каждого сервиса и ключи Kubernetes ConfigMap/Secret
