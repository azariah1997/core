// Package jwtverify is a minimal, provider-neutral JWT/JWKS verifier
// shared by any service that needs to authenticate a bearer token without
// depending on a specific identity provider's admin API - unlike
// core-api's identity.Provider (which also manages identities at the
// provider), this package only answers "is this token valid, and whose
// subject does it carry".
package jwtverify

import (
	"context"
	"fmt"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	// Issuer is the full issuer URL, e.g. http://localhost:8081/realms/core.
	Issuer string
	// JWKSURL is the key set endpoint. If empty, defaults to
	// Issuer + "/protocol/openid-connect/certs" (Keycloak's convention) -
	// pass it explicitly for other providers.
	JWKSURL string
	// Audience, if set, must appear in the token's aud claim, or match
	// azp when aud is absent (common for public OIDC clients).
	Audience string
}

type Claims struct {
	Subject  string
	Username string
	Email    string
}

type Verifier struct {
	cfg  Config
	jwks keyfunc.Keyfunc
}

func New(ctx context.Context, cfg Config) (*Verifier, error) {
	jwksURL := cfg.JWKSURL
	if jwksURL == "" {
		jwksURL = strings.TrimRight(cfg.Issuer, "/") + "/protocol/openid-connect/certs"
	}
	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("load jwks: %w", err)
	}
	return &Verifier{cfg: cfg, jwks: jwks}, nil
}

func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, v.jwks.Keyfunc,
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("validate token: %w", err)
	}
	if v.cfg.Audience != "" && !audienceMatches(claims, v.cfg.Audience) {
		return Claims{}, fmt.Errorf("validate token: audience mismatch")
	}

	sub, _ := claims["sub"].(string)
	username, _ := claims["preferred_username"].(string)
	email, _ := claims["email"].(string)
	return Claims{Subject: sub, Username: username, Email: email}, nil
}

func audienceMatches(claims jwt.MapClaims, want string) bool {
	if aud, err := claims.GetAudience(); err == nil {
		for _, a := range aud {
			if a == want {
				return true
			}
		}
	}
	azp, _ := claims["azp"].(string)
	return azp == want
}
