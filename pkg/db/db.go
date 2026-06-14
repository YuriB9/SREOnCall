package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sre-oncall/pkg/config"
)

// PoolConfig holds the connection-pool tuning applied to every pgxpool.Pool.
// Zero pgxpool defaults (MaxConns = max(4, GOMAXPROCS), no connection recycling)
// are unsafe across five services sharing one Postgres, so NewPool always applies
// these explicit values.
type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// DefaultPoolConfig returns the pool tuning, reading optional overrides from the
// environment (DB_POOL_MAX_CONNS, DB_POOL_MIN_CONNS,
// DB_POOL_MAX_CONN_LIFETIME_SECONDS, DB_POOL_MAX_CONN_IDLE_TIME_SECONDS) and
// falling back to sane defaults sized for several services against one Postgres.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:        int32(config.Int("DB_POOL_MAX_CONNS", 10)),
		MinConns:        int32(config.Int("DB_POOL_MIN_CONNS", 2)),
		MaxConnLifetime: config.DurationSeconds("DB_POOL_MAX_CONN_LIFETIME_SECONDS", 30*time.Minute),
		MaxConnIdleTime: config.DurationSeconds("DB_POOL_MAX_CONN_IDLE_TIME_SECONDS", 5*time.Minute),
	}
}

// NewPool creates and validates a pgxpool.Pool from a connection string.
// The pool is configured with DefaultPoolConfig and validated with a Ping
// before returning.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	pc := DefaultPoolConfig()
	cfg.MaxConns = pc.MaxConns
	cfg.MinConns = pc.MinConns
	cfg.MaxConnLifetime = pc.MaxConnLifetime
	cfg.MaxConnIdleTime = pc.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}
