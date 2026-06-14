# Design — store-layering-and-pool-config

Чисто инфраструктурный рефактор. ADR не заводится: тюнинг пула — рутинное решение,
а перенос кода между пакетами не меняет границ сервисов или транспорта (правило `adr` в
`config.yaml`). Здесь фиксируются только нетривиальные решения и отклонённые альтернативы.

## D4 — конфигурация пула в `pkg/db`

**Решение.** `NewPool(ctx, dsn)` сохраняет сигнатуру; после `pgxpool.ParseConfig` применяются
дефолты пула из `DefaultPoolConfig()`. `PoolConfig` — экспортируемая структура с полями
`MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime`, заполняемая через `pkg/config`
(env с дефолтами): `DB_POOL_MAX_CONNS=10`, `DB_POOL_MIN_CONNS=2`,
`DB_POOL_MAX_CONN_LIFETIME_SECONDS=1800`, `DB_POOL_MAX_CONN_IDLE_TIME_SECONDS=300`.

`HealthCheckPeriod` оставляем на дефолте pgx (1m) — отдельной находки нет, лишний knob не вводим.

**Почему сигнатура не меняется.** Дефолты применяются внутри `NewPool`, поэтому все 5 вызовов
(`*/cmd/server/main.go`) остаются без правок — минимальный дифф для minor-находки. Override —
через env, читается внутри `pkg/db` (тот же модуль `github.com/sre-oncall/pkg`, что и `pkg/config`).

**Отклонённые альтернативы:**
- *Передавать `PoolConfig` параметром в `NewPool`* — потребовало бы трогать 5 `main.go` и протягивать
  поля через 5 сервисных `config.Load()` ради minor-тюнинга. Отклонено: цена выше пользы.
- *Полагаться на `pool_max_conns` в DSN* — текущее (нерабочее) состояние: нигде не задано и не
  задокументировано. Явные дефолты в коде надёжнее.
- *Управлять пулом только через env без экспортируемой структуры* — `PoolConfig`+`DefaultPoolConfig()`
  делают дефолты видимыми и юнит-тестируемыми без живой БД.

**Совместимость.** Схема БД не меняется, миграций нет. Под нагрузкой меняется лишь верхняя граница
соединений (теперь 10/сервис по умолчанию вместо `max(4, GOMAXPROCS)`); при необходимости поднимается
через env без передеплоя кода.

## F2 — вынос слоя persistence/infra из `package main` (ingestion)

**Решение.** Три типа из `cmd/server/main.go` переезжают в `internal/`, выравнивая ingestion
с остальными четырьмя сервисами (`golang-project-layout`: `main` только парсит конфиг и собирает
зависимости):

| Было (в `main`) | Стало | Конструктор |
|---|---|---|
| `pgStore` + SQL `raw_alerts` | `internal/store.Store` | `store.New(pool)` |
| `redisCacheAdapter` (SetNX/Del) | `internal/dedup.RedisCache` | `dedup.NewRedisCache(rdb)` |
| `redisTokenStore` (HGet) | `internal/tokenstore.Store` | `tokenstore.New(rdb)` |

Интерфейсы-потребители не меняются: `handler.Store`, `dedup.Cache`, `middleware.TokenStore` остаются
на месте — новые типы просто реализуют их в своих пакетах («accept interfaces, return structs»).
`RedisCache` кладётся в существующий пакет `dedup` (Redis-реализация его же интерфейса `Cache`),
как и предлагал аудит; токен-стор выделяется в новый `tokenstore`, т.к. логически отдельная роль.

**Почему без дельты спека.** Наблюдаемое поведение (эндпойнты, дедуп, запись `raw_alerts`,
резолв токена) идентично — это перемещение кода, а не изменение гарантий. Прецедент `harden-auth-shell`.
