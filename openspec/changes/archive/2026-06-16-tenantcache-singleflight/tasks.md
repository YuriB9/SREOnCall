# Tasks — tenantcache-singleflight (CH16, находка C7)

Объём строго = находка **C7** из «Матрицы покрытия» (`docs/audit/00-roadmap.md`).
Источник истины: [docs/audit/02-concurrency.md §C7](../../../docs/audit/02-concurrency.md).
Соседние находки (C5/C8 и пр.) — другие чейнджи, не трогаем.

## 1. tenantcache — singleflight против stampede (C7.1)

- [x] 1.1 **C7** — добавить `sf singleflight.Group` в `Cache`, `services/notification/internal/tenantcache/cache.go` ([cache.go:21-26](../../../services/notification/internal/tenantcache/cache.go#L21-L26)).
- [x] 1.2 **C7** — в `Get` обернуть путь промаха в `sf.Do(tenantSlug, fn)`: внутри `fn` сделать `fetcher.GetTenantNotificationConfig` и записать в `data` под `mu`; одновременные промахи по одному ключу схлопываются в один fetch, `services/notification/internal/tenantcache/cache.go` ([cache.go:44-54](../../../services/notification/internal/tenantcache/cache.go#L44-L54)). Сохранить семантику: лок не держится через I/O, ошибка fetch не пишется в `data`.

## 2. tenantcache — вытеснение протухших ключей (C7.2)

- [x] 2.1 **C7** — `New(ctx, fetcher, ttl)`: запустить фоновую горутину периодической чистки `data` от записей с `expiresAt` в прошлом (тикер, интервал ~= ttl), остановка по `ctx.Done()`, `services/notification/internal/tenantcache/cache.go` ([cache.go:28-34](../../../services/notification/internal/tenantcache/cache.go#L28-L34)).
- [x] 2.2 **C7** — приватный `evictExpired()` под `mu`: удалить из `data` записи с истёкшим TTL, `services/notification/internal/tenantcache/cache.go`.

## 3. Разводка в notification main

- [x] 3.1 пробросить `ctx` (рабочий контекст сервиса) в `tenantcache.New(ctx, schedClient, 5*time.Minute)`, чтобы фоновая чистка останавливалась при shutdown, `services/notification/cmd/server/main.go` ([main.go:63](../../../services/notification/cmd/server/main.go#L63)).

## 4. Тесты (новый файл)

- [x] 4.1 **C7** — `cache_test.go`: дедупликация stampede — N параллельных `Get` по одному ключу при промахе → ровно 1 вызов fake-fetcher (счётчик с атомиком/мьютексом), `services/notification/internal/tenantcache/cache_test.go`.
- [x] 4.2 **C7** — тест вытеснения: записать ключ, дождаться протухания + чистки → ключ удалён из `data` (или проверить `evictExpired()` напрямую с инъекцией времени/коротким ttl).
- [x] 4.3 регресс: повторный `Get` в пределах TTL отдаёт кеш без нового fetch; ошибка fetch не кешируется.

## 5. Верификация (Definition of Done, `CHANGE-KICKOFF.md` §3)

- [x] 5.1 `go build ./...`, `go vet ./...`, `go test ./...` в `services/notification`.
- [x] 5.2 **`go test -race ./internal/tenantcache/...`** (конкурентность — singleflight + фоновая чистка) — обязательно по §3 (CH16 — конкурентный чейндж).
- [x] 5.3 `golangci-lint run --new-from-merge-base main` в `services/notification` (с `GOWORK=off`, помодульно — грабли CH01).
- [x] 5.4 `govulncheck ./...` в `services/notification` — чисто.
- [x] 5.5 `go mod tidy` в `services/notification` — без диффа (`x/sync` уже direct).
- [x] 5.6 `/opsx:verify`.

## 6. Хэндофф

- [x] 6.1 `/opsx:archive` (с `--skip-specs` — no-delta).
- [x] 6.2 Обновить `docs/audit/00-roadmap.md`: CH16 → `✅ done` в дашборде (прогресс 15/19) и в строке чейнджа.
- [x] 6.3 `docs/spec-vs-code-audit.md` — обновлять не требуется (поведение/спек не меняются).
