package config

import (
	"fmt"
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

	KeycloakURL           string
	KeycloakRealm         string
	KeycloakAdminUsername string
	KeycloakAdminPassword string
	OpenFGAURL            string
	TemporalAddr          string

	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string

	OpenSearchURL string
	OtelEndpoint  string

	JWTIssuer   string
	JWTAudience string
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

		KeycloakURL:           env("KEYCLOAK_URL", "http://localhost:8081"),
		KeycloakRealm:         env("KEYCLOAK_REALM", "core"),
		KeycloakAdminUsername: env("KEYCLOAK_ADMIN_USERNAME", "admin"),
		KeycloakAdminPassword: env("KEYCLOAK_ADMIN_PASSWORD", "admin"),
		OpenFGAURL:            env("OPENFGA_URL", "http://localhost:8082"),
		TemporalAddr:          env("TEMPORAL_ADDR", "localhost:7233"),

		S3Endpoint:  env("S3_ENDPOINT", "http://localhost:9000"),
		S3Bucket:    env("S3_BUCKET", "core-platform"),
		S3AccessKey: env("S3_ACCESS_KEY", "minio"),
		S3SecretKey: env("S3_SECRET_KEY", "minio123"),

		OpenSearchURL: env("OPENSEARCH_URL", "http://localhost:9200"),
		OtelEndpoint:  env("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),

		JWTIssuer:   env("JWT_ISSUER", "http://localhost:8081/realms/core"),
		JWTAudience: env("JWT_AUDIENCE", "core-platform"),
	}
}

// Validate fails fast when required production configuration is missing or
// still set to a local-development default. It is a no-op outside
// PLATFORM_ENV=production so local development stays a single command.
func (c Config) Validate() error {
	if c.Env != "production" {
		return nil
	}
	var insecure []string
	check := func(name string, value, localDefault string) {
		if value == "" || value == localDefault {
			insecure = append(insecure, name)
		}
	}
	check("POSTGRES_DSN", c.PostgresDSN, "postgres://core:core@localhost:5432/core?sslmode=disable")
	check("REDIS_ADDR", c.RedisAddr, "localhost:6379")
	check("KEYCLOAK_URL", c.KeycloakURL, "http://localhost:8081")
	check("KEYCLOAK_ADMIN_PASSWORD", c.KeycloakAdminPassword, "admin")
	check("JWT_ISSUER", c.JWTIssuer, "http://localhost:8081/realms/core")
	check("S3_SECRET_KEY", c.S3SecretKey, "minio123")
	if len(c.KafkaBrokers) == 0 || c.KafkaBrokers[0] == "localhost:9092" {
		insecure = append(insecure, "KAFKA_BROKERS")
	}
	if len(insecure) > 0 {
		return fmt.Errorf("missing or insecure required production configuration: %s", strings.Join(insecure, ", "))
	}
	return nil
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
