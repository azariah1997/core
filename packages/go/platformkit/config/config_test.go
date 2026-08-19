package config

import (
	"os"
	"regexp"
	"testing"
)

// TestVersionReadsTheRealGitSHA proves gitVersion() actually shells out
// to the real repo, not a hardcoded stand-in - this test runs inside
// this real repo's checkout, so Load().Version must be a real short
// SHA (7+ hex chars), not "dev" (the only fallback value), unless
// BUILD_VERSION happens to be set in the test environment.
func TestVersionReadsTheRealGitSHA(t *testing.T) {
	if os.Getenv("BUILD_VERSION") != "" {
		t.Skip("BUILD_VERSION is set in this environment - gitVersion()'s fallback path isn't being exercised")
	}
	c := Load()
	if matched, _ := regexp.MatchString(`^[0-9a-f]{7,40}$`, c.Version); !matched {
		t.Fatalf("expected a real git short SHA, got %q - is this test running inside a git checkout with git on PATH?", c.Version)
	}
}

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
