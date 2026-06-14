package db

import (
	"testing"
	"time"
)

func TestDefaultPoolConfig_Defaults(t *testing.T) {
	t.Setenv("DB_POOL_MAX_CONNS", "")
	t.Setenv("DB_POOL_MIN_CONNS", "")
	t.Setenv("DB_POOL_MAX_CONN_LIFETIME_SECONDS", "")
	t.Setenv("DB_POOL_MAX_CONN_IDLE_TIME_SECONDS", "")

	got := DefaultPoolConfig()
	want := PoolConfig{
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
	}
	if got != want {
		t.Fatalf("DefaultPoolConfig() = %+v, want %+v", got, want)
	}
}

func TestDefaultPoolConfig_EnvOverride(t *testing.T) {
	t.Setenv("DB_POOL_MAX_CONNS", "25")
	t.Setenv("DB_POOL_MIN_CONNS", "5")
	t.Setenv("DB_POOL_MAX_CONN_LIFETIME_SECONDS", "600")
	t.Setenv("DB_POOL_MAX_CONN_IDLE_TIME_SECONDS", "120")

	got := DefaultPoolConfig()
	want := PoolConfig{
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: 10 * time.Minute,
		MaxConnIdleTime: 2 * time.Minute,
	}
	if got != want {
		t.Fatalf("DefaultPoolConfig() = %+v, want %+v", got, want)
	}
}

func TestDefaultPoolConfig_InvalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("DB_POOL_MAX_CONNS", "not-a-number")

	if got := DefaultPoolConfig().MaxConns; got != 10 {
		t.Fatalf("MaxConns with invalid env = %d, want default 10", got)
	}
}
