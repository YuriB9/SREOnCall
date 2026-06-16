## 1. ingestion — пайплайн дедупликации (P2, Redis)

- [x] 1.1 P2 — `dedup.Cache.Apply(ctx, setKeys, val, ttl, delKeys) ([]bool, error)`: пайплайн SETNX+DEL в один round-trip; реализация в `RedisCache` через `redis.Pipeline()` — `services/ingestion/internal/dedup/dedup.go`, `services/ingestion/internal/dedup/redis.go`
- [x] 1.2 P2 — `Deduplicator.Classify(ctx, []domain.Alert) ([]bool, error)`: firing→SETNX, resolved→Clear через `Apply`; поалертные метрики `dedupHits`/`dedupMisses` сохранены; `IsDuplicate`/`Clear` оставлены для регрессий и отката — `services/ingestion/internal/dedup/dedup.go:46-67`

## 2. ingestion — батч-сохранение raw_alerts + единый Marshal (P2 + P5)

- [x] 2.1 P2/P5 — тип `store.RawAlert{Alert domain.Alert; Payload json.RawMessage; Deduplicated bool}` и `Store.SaveRawAlerts(ctx, []RawAlert) error` через `pgx.Batch`; переиспользует готовый `Payload`, не маршалит повторно — `services/ingestion/internal/store/store.go:24-37`
- [x] 2.2 P2 — обновить интерфейс `handler.Store` (`SaveRawAlert` → `SaveRawAlerts`) и разводку в `main.go` (store уже совместим) — `services/ingestion/internal/handler/handler.go:26-29`, `services/ingestion/cmd/server/main.go:78`

## 3. ingestion — группировка публикаций (P2) + единый payload (P5)

- [x] 3.1 P5 — `publisher.PublishAlertPayload(ctx, tenantID string, payload json.RawMessage) error`: `Wrap` готового payload без реэнкода `Alert`; публикация на переиспользуемом канале (CH14) — `services/ingestion/internal/publisher/publisher.go:24-31`
- [x] 3.2 P2 — обновить интерфейс `handler.Publisher` под публикацию готового payload — `services/ingestion/internal/handler/handler.go:21-24`

## 4. ingestion — переписать processAlerts (P2, ядро)

- [x] 4.1 P2/P5 — `processAlerts`: фазы Marshal(один раз) → Classify(пайплайн) → SaveRawAlerts(батч) → публикация неподавленных; откат дедуп-ключа `Clear` при ошибке публикации; порядок алертов сохранён; ответ `503` при ошибке — `services/ingestion/internal/handler/handler.go:44-86`
- [x] 4.2 P2 — удалить/свернуть поалертный `processOne`, сохранив поалертный инкремент `alertsReceived` — `services/ingestion/internal/handler/handler.go:53-86`

## 5. incident — multi-row upsert лейблов (P3)

- [x] 5.1 P3 — `mergeLabels`: один `INSERT ... SELECT FROM unnest($2::text[], $3::text[]) ON CONFLICT (incident_id, key) DO UPDATE` вместо цикла построчных INSERT; покрывает `MergeLabels` и `CreateIncidentTx` — `services/incident/internal/store/store.go:441-457`

## 6. Тесты (обновление и регрессии)

- [x] 6.1 Обновить моки `webhook_test.go`: `noopStore`→`SaveRawAlerts`, `capturePublisher`→новый метод, `memCache`→`Apply` — `services/ingestion/internal/handler/webhook_test.go`
- [x] 6.2 Тест: тело с дублем fingerprint в одном батче → опубликован один раз (батч-дедуп) — `services/ingestion/internal/handler/webhook_test.go`
- [x] 6.3 Тест: порядок опубликованных алертов соответствует телу вебхука — `services/ingestion/internal/handler/webhook_test.go`
- [x] 6.4 Тест `Classify`: смешанные firing/resolved → корректные флаги подавления и счётчики — `services/ingestion/internal/dedup/dedup_test.go`
- [x] 6.5 Тест `mergeLabels`: несколько лейблов + upsert-конфликт (обновление значения) — `services/incident/internal/store/`

## 7. Бенчмарки (benchstat обязателен — правило аудита)

- [x] 7.1 `BenchmarkProcessAlerts` (ingestion, live Redis+PG, `b.Skip` без инфры — паттерн CH14) — `services/ingestion/internal/handler/`
- [x] 7.2 `BenchmarkMergeLabels` (incident, live PG, `b.Skip` без инфры) — `services/incident/internal/store/`
- [x] 7.3 Прогнать benchstat до/после, приложить `benchstat.txt` в директорию чейнджа

## 8. Верификация

- [x] 8.1 `go build ./...` и `go vet ./...` во всех затронутых модулях (ingestion, incident, pkg)
- [x] 8.2 `go test ./...` (ingestion, incident); `go test -race` для ingestion handler/dedup
- [x] 8.3 `golangci-lint run --new-from-merge-base main` помодульно (`GOWORK=off`, конфиг абсолютным путём); `govulncheck ./...`; `go mod tidy` без диффа
- [x] 8.4 `/opsx:verify` → обновить статус CH15 в `docs/audit/00-roadmap.md` (дашборд + строка чейнджа)
