## Context

Экран `/[tenant]/settings/notifications` рендерится единственным компонентом `NotificationConfigSection` ([NotificationConfigPage.tsx](../../../frontend/src/pages/settings/NotificationConfigPage.tsx)), который выводит три поля (`mattermost_webhook_url`, `mattermost_channel`, `smtp_from`) одной плоской формой с одной кнопкой «Сохранить».

Per-tenant конфигурация хранится в таблице `tenant_notification_config` (схема scheduling). Модель `store.NotificationConfig` ([store.go:394](../../../services/scheduling/internal/store/store.go#L394)) и хендлеры GET/PUT ([handler.go:635](../../../services/scheduling/internal/handler/handler.go#L635)) знают только эти три поля. `mattermost_webhook_url` маскируется в GET и трактуется как «не менять» при пустом PUT.

SMTP-транспорт (host/port/username/password) — глобальный env notification-сервиса ([config.go:24](../../../services/notification/internal/config/config.go#L24)); per-tenant из email сейчас только `smtp_from`, который notifier применяет как override ([notifier.go:191](../../../services/notification/internal/notifier/notifier.go#L191)). Тема письма и отсутствие Reply-To захардкожены в `dispatcher.Email.Send` ([email.go](../../../services/notification/internal/dispatcher/email.go)).

## Goals / Non-Goals

**Goals:**
- Разделить форму на две независимые секции с раздельным сохранением: «Mattermost» и «Email».
- Добавить per-tenant email-настройки без секретов: `email_enabled`, `email_reply_to`, `email_subject_prefix`.
- Применять новые поля при отправке email в notification-сервисе.
- Сохранить полную обратную совместимость (старые записи БД и старые тела запросов).

**Non-Goals:**
- Per-tenant SMTP host/port/username/password (секреты, маскирование, отдельное хранилище) — сознательно вне объёма (выбор пользователя: «только UI + лёгкие поля»).
- HTML-письма, шаблонизация тела, локализация писем.
- Изменение Mattermost-логики и rate-limiting.

## Decisions

### Решение 1: Две секции — два независимых PUT, один эндпоинт

Каждая секция («Mattermost», «Email») получает свою кнопку «Сохранить» и шлёт частичный `PUT /notification-config` только со своими полями. Эндпоинт остаётся один.

Это требует, чтобы PUT был **частичным по семантике для каждой группы полей**, а не только для `mattermost_webhook_url`. Сейчас PUT принимает `store.NotificationConfig` и перезаписывает строку целиком (`UpsertNotificationConfig` делает `ON CONFLICT ... SET все поля`). Если секция Email пришлёт только email-поля, Mattermost-канал затрётся пустыми значениями.

**Выбор:** перевести `PutTenantNotificationConfig` на чтение тела в указатели (`*string`/`*bool`) и обновлять только присутствующие поля — текущая запись читается, неуказанные поля сохраняются. Это обобщает уже существующий приём «пустой webhook = оставить текущий» на все поля и делает раздельное сохранение секций корректным.

- _Альтернатива A — два разных эндпоинта (`.../notification-config/mattermost` и `/email`)_: больше маршрутов, дублирование auth, расхождение со спекой одного ресурса. Отклонено.
- _Альтернатива B — одна кнопка «Сохранить» на весь экран_: проще, но противоречит цели «две независимые секции». Отклонено.

### Решение 2: Схема `tenant_notification_config` — 3 новые колонки с DEFAULT

Новая миграция в `services/scheduling/migrations/` (`000002_*`):

```sql
ALTER TABLE tenant_notification_config
  ADD COLUMN mattermost_enabled   boolean NOT NULL DEFAULT true,
  ADD COLUMN email_enabled        boolean NOT NULL DEFAULT true,
  ADD COLUMN email_reply_to       text    NOT NULL DEFAULT '',
  ADD COLUMN email_subject_prefix text    NOT NULL DEFAULT '';
```

`DEFAULT true`/`''` обеспечивает обратную совместимость: существующие тенанты получают оба канала включёнными без Reply-To и префикса. Не **BREAKING**.

### Решение 3: Поля DTO и проброс в notification-сервис

- `store.NotificationConfig` + GET/Upsert: добавить `EmailEnabled bool`, `EmailReplyTo string`, `EmailSubjectPrefix string`. Эти поля не маскируются (не секреты), отдаются в GET как есть.
- `schedclient.Config` (notification) ([client.go](../../../services/notification/internal/schedclient/client.go)): добавить те же поля.
- `dispatcher.EmailMessage` расширить полями `ReplyTo` и `SubjectPrefix`; в `Send` префикс добавляется в начало темы, а `Reply-To` — в заголовки письма (только если непустой).
- `notifier`: ветка `ChannelEmail` пропускает отправку при `cfg.EmailEnabled == false` (лог info, без записи `failed`); заполняет `ReplyTo`/`SubjectPrefix` из cfg. Ветка `ChannelMattermost` симметрично пропускает отправку при `cfg.MattermostEnabled == false` (лог info, без записи `failed`); существующая проверка отсутствующего/маскированного webhook (запись `failed`) сохраняется для включённого канала. Та же проверка `mattermost_enabled` применяется в `NotifyExhausted`.

### Решение 6: Симметричный тумблер Mattermost (`mattermost_enabled`)

Чтобы обе секции UI выглядели и вели себя единообразно (выбор пользователя — «полная симметрия тумблеров»), Mattermost получает явный флаг `mattermost_enabled`, зеркальный `email_enabled`, а не неявное «есть/нет webhook». Это даёт админу неразрушающее отключение канала (webhook сохраняется). Различие семантики сохраняется на уровне логирования: `enabled=false` — это намеренное info-отключение, тогда как `enabled=true` без webhook остаётся ошибкой доставки (`failed`). На фронте при `enabled=true` и пустом webhook показывается мягкое предупреждение.

- _Альтернатива — оставить Mattermost без тумблера (асимметрия)_: проще, но непоследовательно для пользователя; отклонено в пользу симметрии.
- _Альтернатива — тумблер «выключить» = очистить webhook_: разрушительно (теряется сохранённый URL); отклонено в пользу отдельного флага.

### Решение 4: GET возвращает дефолтный конфиг вместо 404 (фикс «незаполненный конфиг»)

Сейчас при отсутствии строки: `store.GetNotificationConfig` → `ErrNotFound` → `GetTenantNotificationConfig` отдаёт **404** → `schedclient` маппит 404 в `nil` → `tenantcache` кэширует `nil` на TTL (5 минут, [main.go:61](../../../services/notification/cmd/server/main.go#L61)). Итог: после первого сохранения настроек notification до 5 минут продолжает видеть пустой конфиг.

**Выбор:** в `GetTenantNotificationConfig` при `ErrNotFound` отдавать **HTTP 200 с дефолтным конфигом** (`tenant_id` = slug, `email_enabled=true`, остальные поля — пустые строки) по каноническому маршруту. Тогда:
- `schedclient` всегда получает 200 → в кэш кладётся реальный (дефолтный) конфиг, а не `nil`;
- `email_enabled` имеет корректный дефолт `true` для незаполненных тенантов;
- фронтенд тоже выигрывает: `useNotificationConfig` сейчас падает на 404, а станет получать дефолтный объект для предзаполнения формы.

Дополнительно в notifier ветка `ChannelEmail` при `cfg == nil` (сетевой сбой и т.п.) ДОЛЖНА считать email включённым — отсутствие конфига не должно молча гасить email.

- _Альтернатива — автосоздание строки при создании тенанта_: чище концептуально, но трогает поток создания тенанта и не покрывает уже существующие пустые тенанты. Отклонено в пользу дефолта на чтении (работает и для легаси-тенантов).

Особый случай 404/405 для **неканонических** маршрутов (`/{tenant}/notification-config`) сохраняется без изменений — это про защиту от обходных путей, а не про отсутствие строки.

### Решение 5: Валидация на фронте

`email_reply_to` валидируется тем же `isValidEmail`, что и `smtp_from`. `email_subject_prefix` — свободный текст (разумно ограничить длину, напр. 64 символа). `email_enabled` — чекбокс/toggle.

## Risks / Trade-offs

- **Переход PUT на частичное обновление всех полей может скрыть очистку поля** (пользователь не сможет очистить `mattermost_channel`, отправив пустую строку, т.к. пустое = «не менять») → для строковых не-секретных полей (`mattermost_channel`, `smtp_from`, `email_reply_to`, `email_subject_prefix`) сохраняем семантику «прислано → перезаписать (в т.ч. пустым)», а «не прислано → оставить». Поэтому тело читаем в указатели: `nil` (поля нет) ≠ `""` (явно очищено). Только `mattermost_webhook_url` остаётся особым (пустая строка = «не менять», т.к. маскируется).
- **Рассинхрон notifier с маскированием** → email-поля не маскируются, отдаются полностью и в пользовательский GET, и в сервисный; рисков утечки секретов нет.
- **Старые события без полей инцидента** → поведение fallback в `Send` сохраняется; префикс темы добавляется и к fallback-теме.
- **Остаточная staleness кэша (до 5 минут)** → даже после фикса 404→200 кэш notification держит конфиг до истечения TTL (5 мин), поэтому изменения настроек применяются с задержкой. Это TTL-задержка, а не «обрыв»: дефолтный конфиг уже работоспособен (email через глобальный SMTP). Инвалидация кэша между сервисами — отдельная задача вне объёма; задержка приемлема и ограничена TTL.

## Migration Plan

1. Применить миграцию scheduling (автоматически при старте сервиса через golang-migrate).
2. Выкатить scheduling (GET/PUT с новыми полями) — обратно совместимо со старым фронтом и старым notification.
3. Выкатить notification (чтение и применение новых полей).
4. Выкатить фронтенд (две секции + поля).

Откат: миграция down удаляет три колонки; промежуточные версии сервисов игнорируют неизвестные поля (JSON), поэтому порядок выката некритичен.

## Open Questions

- Нужно ли ограничение максимальной длины `email_subject_prefix` на бэкенде (валидация 422), или достаточно мягкого ограничения на фронте? Предлагается мягкое ограничение на фронте (64 символа), без жёсткой валидации на бэке.
