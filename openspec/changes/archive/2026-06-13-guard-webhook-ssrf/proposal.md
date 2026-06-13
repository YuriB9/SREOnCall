## Why

Аудит безопасности (`docs/audit/05-security.md`, находка **S2**, major/Medium — SSRF) выявил, что
URL входящего вебхука Mattermost полностью подконтролен тенант-админу и нигде не ограничивается
по адресу назначения:

- **На записи** (`PUT /api/schedules/v1/tenants/{slug}/notification-config`) сохраняется любая
  непустая строка — валидации хоста нет ([scheduling/handler.go:705-707](services/scheduling/internal/handler/handler.go#L705-L707)).
- **На отправке** `webhookURLUsable` проверяет только синтаксис (scheme/host/path непустые), но не
  ограничивает адрес ([notification/notifier.go:317-326](services/notification/internal/notifier/notifier.go#L317-L326)),
  после чего диспетчер делает `POST` на этот URL.

Аутентифицированный тенант-админ может указать `http://169.254.169.254/latest/meta-data/...`,
`http://localhost:8082/...` или адрес внутреннего k8s-сервиса и заставить notification-под обращаться
во внутреннюю сеть от своего имени (blind SSRF: разведка сети, обращение к cloud-metadata).

## What Changes

- **S2 — общий барьер `pkg/ssrf`.** Новый пакет с `ValidateURL(raw)` (требует схему `https`, резолвит
  хост и блокирует приватные/loopback/link-local/unspecified/multicast/ULA диапазоны: RFC 1918, `127/8`,
  `169.254/16`, `::1`, `fc00::/7` и т.п.) и `GuardedDialContext` — обёрткой над `net.Dialer`, проверяющей
  фактический IP в момент дозвона (закрывает TOCTOU/DNS-rebinding и редиректы на приватные адреса).
- **S2 — валидация на записи (scheduling).** В `PutTenantNotificationConfig` при непустом
  `mattermost_webhook_url` вызывается `ssrf.ValidateURL`; небезопасный URL отклоняется с **HTTP 422**.
  Пустой/отсутствующий URL обрабатывается как раньше (сохраняем прежнее значение).
- **S2 — фильтр на отправке (notification, defense in depth).** HTTP-клиент Mattermost-диспетчера
  получает `GuardedDialContext`: приватные адреса блокируются на дозвоне даже для ранее сохранённых
  URL и для редиректов. Сбой отправки идёт по существующему пути `failed` в журнале доставки.

Стратегия — **блок приватных диапазонов** (не allowlist доменов): не требует ведения списка доменов и
работает с self-hosted Mattermost на произвольных публичных доменах (см. ADR-0013).

Расширение валидации, **не BREAKING**: легитимные публичные https-URL Mattermost проходят как раньше;
отклоняются только небезопасные адреса, сохранять которые и не следовало.

## Capabilities

### New Capabilities
<!-- Новых capability не вводится. -->

### Modified Capabilities

- **tenant-management** — «Конфигурация уведомлений тенанта»: PUT валидирует `mattermost_webhook_url`
  (только https + не приватный адрес), небезопасный URL → 422.
- **notification-dispatch** — «Отправка уведомлений в Mattermost через входящий вебхук»: отправка на
  приватный/не-https адрес блокируется (defense in depth), доставка помечается `failed`.

## Impact

- **Затронутые сервисы:** `scheduling` (валидация на записи, `internal/handler/handler.go`),
  `notification` (guarded dialer, `internal/dispatcher/mattermost.go`).
- **Общий код:** новый `pkg/ssrf` (`ValidateURL`, `GuardedDialContext`, sentinel-ошибки).
- **События RabbitMQ:** не затрагиваются.
- **HTTP-API:** `PUT /api/schedules/v1/tenants/{slug}/notification-config` теперь может вернуть **422**
  на небезопасный `mattermost_webhook_url`. Контракт успешного пути не меняется — **не BREAKING**.
- **Деплой:** новых env не вводится. Уже сохранённые небезопасные URL перестанут доставляться (блок на
  дозвоне) — это цель находки.
- **Документация:** ADR-0013 (защита от SSRF в тенант-задаваемых вебхуках), обновление
  `docs/spec-vs-code-audit.md` (S2).
- **Тесты:** юнит-тесты `pkg/ssrf` (валидатор: публичный https — ok; http/приватные/loopback/metadata —
  reject) и обновление тестов scheduling-хендлера на путь 422.
