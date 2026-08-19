package config

import "testing"

func TestValidatePassesOutsideProduction(t *testing.T) {
	c := Load()
	c.Env = "local"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected local env to skip validation, got %v", err)
	}
}

func TestValidateFailsFastOnDefaultProductionConfig(t *testing.T) {
	c := Load()
	c.Env = "production"
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error when production still uses local defaults")
	}
}

func TestValidatePassesWhenProductionConfigOverridden(t *testing.T) {
	c := Load()
	c.Env = "production"
	c.PostgresDSN = "postgres://prod-user:secret@prod-host:5432/core"
	c.RedisAddr = "prod-redis:6379"
	c.KeycloakURL = "https://auth.example.com"
	c.KeycloakAdminPassword = "real-admin-secret"
	c.JWTIssuer = "https://auth.example.com/realms/core"
	c.S3SecretKey = "real-secret"
	c.StripeWebhookSecret = "whsec_real_production_secret"
	c.KafkaBrokers = []string{"prod-kafka:9092"}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected overridden production config to pass, got %v", err)
	}
}
