// Package config provides primitives for reading configuration from the
// environment with defaults, replacing the getenv/envOr/getenvInt/envDurSec
// helpers that had been copied into every service's config package under
// different names.
package config

import (
	"os"
	"strconv"
	"time"
)

// String returns the value of the environment variable key, or def when it is
// unset or empty.
func String(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Int returns the integer value of the environment variable key, or def when it
// is unset or not a valid integer. Unlike a fmt.Sscanf-based helper, a legitimate
// "0" is accepted.
func Int(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// DurationSeconds interprets the environment variable key as a whole number of
// seconds, returning def when it is unset or not a valid integer.
func DurationSeconds(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return time.Duration(n) * time.Second
}
