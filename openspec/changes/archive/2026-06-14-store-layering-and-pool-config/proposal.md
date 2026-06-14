## Why

Два структурных долга из аудита (`docs/audit`, находки F2 и D4):

- **F2** — у `ingestion` слой персистентности и инфраструктурные адаптеры живут прямо в
  `package main` ([ingestion/cmd/server/main.go:127-164](../../../services/ingestion/cmd/server/main.go#L127-L164)):
  `pgStore` с SQL `INSERT INTO ingestion.raw_alerts`, `redisCacheAdapter`, `redisTokenStore`.
  Это единственный сервис, выбивающийся из общей раскладки `internal/store`; SQL-схема непокрываема
  тестами пакета, а зависимости не видны из `internal/handler`.
- **D4** — [pkg/db.NewPool](../../../pkg/db/db.go#L12-L25) не задаёт ни одного параметра пула
  (`MaxConns`/`MinConns`/`MaxConnLifetime`/`MaxConnIdleTime`), полагаясь на дефолты pgx
  (`max(4, GOMAXPROCS)`, без рециклинга соединений). Пять сервисов под нагрузкой могут
  непредсказуемо упереться в `max_connections` Postgres, а долгоживущие соединения не
  рециклируются (проблема за PgBouncer / при failover реплики).

## What Changes

- **F2** — вынести из `ingestion/cmd/server/main.go` в `internal/`, оставив в `main` только конструкторы:
  - `pgStore` + SQL `raw_alerts` → новый пакет `internal/store`;
  - `redisCacheAdapter` (SetNX/Del) → `internal/dedup` (Redis-реализация интерфейса `Cache`);
  - `redisTokenStore` (HGet) → новый пакет `internal/tokenstore`.
- **D4** — `pkg/db`: разумные дефолты пула в коде (`MaxConns=10`, `MinConns=2`,
  `MaxConnLifetime=30m`, `MaxConnIdleTime=5m`) с возможностью override из окружения
  (`DB_POOL_*`). Сигнатура `NewPool(ctx, dsn)` **сохраняется** — дефолты применяются внутри,
  поэтому четыре остальных сервиса править не нужно.

Изменения API и wire-формата событий нет; **не BREAKING**.

## Capabilities

### New Capabilities

Нет. Чистый infra/layering-рефактор без новых продуктовых capability.

### Modified Capabilities

Нет. Наблюдаемое поведение продуктовых capability не меняется (прецедент `harden-auth-shell`):
тюнинг пула и перенос кода между пакетами не затрагивают гарантии фич. Дельта-спеков нет.

## Impact

- **Сервисы:**
  - **ingestion** — перенос `pgStore`/Redis-адаптеров в `internal/{store,dedup,tokenstore}`;
    поведение HTTP-эндпойнтов, дедупликации и записи `raw_alerts` без изменений.
  - **incident / escalation / scheduling / notification** — косвенно: все получают
    сконфигурированный пул соединений через общий `pkg/db.NewPool` (новые env `DB_POOL_*`).
- **События RabbitMQ:** не затрагиваются.
- **API / wire-формат:** не меняются, **не BREAKING**.
- **Конфигурация:** новые опциональные env-переменные `DB_POOL_MAX_CONNS`, `DB_POOL_MIN_CONNS`,
  `DB_POOL_MAX_CONN_LIFETIME_SECONDS`, `DB_POOL_MAX_CONN_IDLE_TIME_SECONDS` (есть дефолты).
- **Спеки:** дельты нет — архивация с `--skip-specs`.
