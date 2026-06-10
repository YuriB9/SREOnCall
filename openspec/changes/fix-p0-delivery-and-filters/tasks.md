# Задачи: Устранение P0-дефектов доставки уведомлений и фильтрации

## 1. Признак сервисной аутентификации в pkg/auth (баги 2.1, 2.2)

- [x] 1.1 В `pkg/auth`: при проходе по `X-Admin-Key` класть в контекст признак способа аутентификации (`service`), при JWT — `user`; добавить функцию чтения признака из контекста
- [x] 1.2 Юнит-тесты middleware: запрос с верным ключом → `service`; с JWT → `user`; неверный ключ без JWT → 401

## 2. Сервисный ключ в schedclient (баг 2.1)

- [x] 2.1 `services/escalation/internal/schedclient/client.go`: принять `adminKey` в конструкторе, отправлять `X-Admin-Key` в каждом запросе
- [x] 2.2 `services/notification/internal/schedclient/client.go`: то же самое
- [x] 2.3 `services/escalation/internal/config`, `services/notification/internal/config`: параметр `SCHEDULING_ADMIN_KEY`; прокинуть в конструкторы клиентов в `cmd/server/main.go`
- [x] 2.4 `services/escalation/internal/escalator/escalator.go` (`triggerTier`): поднять уровень лога сбоя резолва дежурного до `error` (публикация с пустым `oncall_user_id` не должна быть тихой)
- [x] 2.5 Тесты клиентов: заголовок присутствует; 401 от scheduling возвращается ошибкой, а не пустым результатом

## 3. Условное маскирование конфига уведомлений (баг 2.2)

- [x] 3.1 `services/scheduling/internal/handler/handler.go` (`GetTenantNotificationConfig`): маскировать `mattermost_webhook_url` только для `user`-запросов; для `service` — полный URL; при неопределённом признаке — маскировать
- [x] 3.2 `services/notification/internal/notifier/notifier.go`: перед Mattermost-отправкой отбрасывать маскированный/пустой URL (нет пути после хоста) с записью `failed` в журнал и error-логом
- [x] 3.3 Тесты handler: GET с JWT → маскированный URL; GET с `X-Admin-Key` → полный URL

## 4. Защита webhook URL при сохранении (баг 2.4)

- [x] 4.1 `services/scheduling/internal/handler/handler.go` (`PutTenantNotificationConfig`): при пустом/отсутствующем `mattermost_webhook_url` сохранять текущее значение из БД; остальные поля обновлять
- [x] 4.2 `frontend/src/pages/TenantSettingsPage.tsx`: не включать `mattermost_webhook_url` в тело PUT, если поле не заполнялось
- [x] 4.3 Тесты: PUT с пустым URL при сохранённом непустом → URL не изменился; PUT с непустым URL → заменился

## 5. Мульти-статусный фильтр инцидентов (баг 2.3)

- [ ] 5.1 `services/incident/internal/handler/handler.go`: разобрать `status` по запятой, валидировать каждое значение по `open|acknowledged|resolved`, при недопустимом — HTTP 400
- [ ] 5.2 `services/incident/internal/store/store.go` (`ListIncidents`): фильтр `i.status = ANY($n)` по срезу статусов
- [ ] 5.3 Тесты store/handler: один статус; два статуса; недопустимое значение → 400

## 6. Деплой

- [ ] 6.1 `deploy/k8s`: завести Secret с `ADMIN_KEY`/`SCHEDULING_ADMIN_KEY`, подключить к scheduling (проверка), escalation и notification (использование); убрать ключ из configmap, если он там появлялся
- [ ] 6.2 `docker-compose.yaml`: задать ключ сервисам для локального паритета с k8s (опционально, поведение без JWKS не меняется)

## 7. Проверка

- [ ] 7.1 Прогнать `go build ./...` и `go test ./...` по затронутым сервисам; `tsc -b` для фронтенда
- [ ] 7.2 Сквозная проверка в k8s-конфигурации (JWKS включён): инцидент → эскалация tier 1 → событие с непустым `oncall_user_id` → email и Mattermost доставлены на полный URL
- [ ] 7.3 Ручная проверка UI: фильтр «Открытые + Подтверждённые» возвращает данные; сохранение настроек без ввода URL не ломает Mattermost
