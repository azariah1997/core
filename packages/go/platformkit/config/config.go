package config

import (
	"os"
	"strings"
)

type Config struct {
	Env          string
	PlatformName string
	HTTPAddr     string
	RealtimeAddr string
	WorkerAddr   string
	PostgresDSN  string
	RedisAddr    string
	KafkaBrokers []string
}

func Load() Config {
	return Config{
		Env:          env("PLATFORM_ENV", "local"),
		PlatformName: env("PLATFORM_NAME", "core-platform"),
		HTTPAddr:     env("HTTP_ADDR", ":8080"),
		RealtimeAddr: env("REALTIME_ADDR", ":8090"),
		WorkerAddr:   env("WORKER_HTTP_ADDR", ":8091"),
		PostgresDSN:  env("POSTGRES_DSN", "postgres://core:core@localhost:5432/core?sslmode=disable"),
		RedisAddr:    env("REDIS_ADDR", "localhost:6379"),
		KafkaBrokers: strings.Split(env("KAFKA_BROKERS", "localhost:9092"), ","),
	}
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
