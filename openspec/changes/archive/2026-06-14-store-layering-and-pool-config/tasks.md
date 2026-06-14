# Tasks — store-layering-and-pool-config

Каждая задача привязана к находке аудита и `file:line` (источник истины — `docs/audit`).

## 1. D4 — конфигурация пула соединений (`pkg/db`)

- [x] 1.1 **D4** — `PoolConfig` + `DefaultPoolConfig()` (env `DB_POOL_MAX_CONNS`/`DB_POOL_MIN_CONNS`/
  `DB_POOL_MAX_CONN_LIFETIME_SECONDS`/`DB_POOL_MAX_CONN_IDLE_TIME_SECONDS` через `pkg/config`,
  дефолты 10/2/30m/5m), применить в `NewPool` после `ParseConfig` — `pkg/db/db.go:12-25`.
- [x] 1.2 **D4** — юнит-тест на `DefaultPoolConfig` (дефолты + env-override, без живой БД) —
  `pkg/db/db_test.go`.

## 2. F2 — вынос persistence/infra из `package main` (ingestion)

- [x] 2.1 **F2** — новый `internal/store/store.go`: `Store` + `New(pool)` + `SaveRawAlert`
  (перенос `pgStore` и SQL `raw_alerts` из `services/ingestion/cmd/server/main.go:151-164`).
- [x] 2.2 **F2** — `internal/dedup/redis.go`: `RedisCache` + `NewRedisCache(rdb)` (SetNX/Del)
  как Redis-реализация `dedup.Cache` (перенос `redisCacheAdapter` из `main.go:129-137`).
- [x] 2.3 **F2** — новый `internal/tokenstore/tokenstore.go`: `Store` + `New(rdb)` + `GetTenantID`
  (перенос `redisTokenStore` из `main.go:139-147`).
- [x] 2.4 **F2** — `services/ingestion/cmd/server/main.go:80-84,127-164`: удалить типы-адаптеры,
  развести зависимости через новые конструкторы; убрать ставшие лишними импорты
  (`encoding/json`, `pgxpool`, `goredis`, `domain`).

## 3. Верификация

- [x] 3.1 `go build ./...`, `go vet ./...`, `go test ./...` — все 6 модулей (особенно `pkg`, ingestion).
- [x] 3.2 `golangci-lint run --new-from-merge-base main` помодульно (`GOWORK=off`, конфиг абс. путём)
  по задетым модулям (`pkg`, `services/ingestion`) — 0 new issues.
- [x] 3.3 `go mod tidy` в задетых модулях — без диффа.
- [x] 3.4 Обновить статус CH09 в `docs/audit/00-roadmap.md` (дашборд `✅` + строка чейнджа).
- [x] 3.5 `docs/spec-vs-code-audit.md` — обновлять не требуется (нет дельты спека; находки F2/D4
  не входят в матрицу спек↔код).
