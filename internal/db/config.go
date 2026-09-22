package db

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 2
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
)

// PoolOptions tunes database/sql pool behavior via GORM.
type PoolOptions struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	SearchPath      string
}

// PoolOptionsFromEnv loads pool settings from environment variables.
func PoolOptionsFromEnv() PoolOptions {
	return PoolOptions{
		MaxOpenConns:    envInt("SUDOSTREAM_DB_MAX_CONNS", defaultMaxOpenConns),
		MaxIdleConns:    envInt("SUDOSTREAM_DB_MIN_CONNS", defaultMaxIdleConns),
		ConnMaxLifetime: envDuration("SUDOSTREAM_DB_MAX_CONN_LIFETIME", defaultConnMaxLifetime),
		ConnMaxIdleTime: envDuration("SUDOSTREAM_DB_MAX_CONN_IDLE_TIME", defaultConnMaxIdleTime),
	}
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}

	return parsed
}
