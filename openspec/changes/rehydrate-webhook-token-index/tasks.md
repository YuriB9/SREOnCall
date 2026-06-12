## 1. Сторадж: чтение хэшей токенов

- [x] 1.1 Добавить в `services/scheduling/internal/store/store.go` метод `ListWebhookTokenHashes(ctx) ([]TokenHashEntry, error)`, возвращающий пары `(token_hash, tenant_id)` по всем строкам `scheduling.tenant_webhook_tokens`
- [x] 1.2 Объявить вспомогательный тип результата (`TokenHashEntry{Hash, TenantID string}`) или эквивалент

## 2. Token index: массовая загрузка

- [x] 2.1 Добавить в `services/scheduling/internal/tokenindex/index.go` метод `SetMany(ctx, entries)` с записью `HSET oncall:tokens:{hash} tenant_id {slug}` для каждой пары (через Redis pipeline)
- [x] 2.2 Обеспечить идемпотентность: повторный вызов приводит к тому же состоянию индекса

## 3. Регидрация при старте scheduling

- [x] 3.1 В `services/scheduling/cmd/server/main.go` после `tidx = tokenindex.New(rdb)` (и только при `rdb != nil`) прочитать хэши через `ListWebhookTokenHashes` и загрузить их через `SetMany`
- [x] 3.2 Логировать число восстановленных токенов (info) и пропуск при недоступном Redis (warning); ошибку чтения/записи логировать как warning без `os.Exit`
- [x] 3.3 Убедиться, что регидрация выполняется до старта HTTP-сервера (`ListenAndServe`)

## 4. Тесты

- [x] 4.1 Юнит/интеграционный тест `tokenindex.SetMany`: записывает все пары, идемпотентен при повторе (реальный Redis из docker-compose)
- [x] 4.2 Интеграционный тест стораджа `ListWebhookTokenHashes`: создаёт токены и возвращает корректные пары `(hash, tenant_id)`
- [x] 4.3 Тест сценария регидрации: после очистки Redis и «старта» (вызова регидрации) ключ `oncall:tokens:{hash}` восстановлен и резолвится в нужный tenant

## 5. Проверка и документация

- [x] 5.1 Ручная проверка: сбросить Redis (`FLUSHALL`/рестарт контейнера), запустить scheduling, отправить вебхук ingestion с ранее выданным токеном — ожидать HTTP 200, а не 401
- [x] 5.2 Обновить `docs/spec-vs-code-audit.md` при необходимости (сверка требования `tenant-management` с реализацией)
