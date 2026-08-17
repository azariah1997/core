// Package keycloak is the Keycloak-backed identity.Provider: real JWT/JWKS
// token verification, plus admin-API-backed identity management. Other
// providers (Google, Apple, Microsoft, passkeys) are additional
// implementations of the same identity.Provider interface, not a rewrite of
// this one.
package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/example/core-platform/backend/core-api/internal/identity"
)

type Config struct {
	BaseURL  string // e.g. http://localhost:8081
	Realm    string // e.g. core
	Audience string // expected aud, or azp when aud is absent (Keycloak's default for public clients)

	AdminUsername string
	AdminPassword string
}

type Provider struct {
	cfg    Config
	issuer string
	jwks   keyfunc.Keyfunc
	http   *http.Client
}

func New(ctx context.Context, cfg Config) (*Provider, error) {
	issuer := strings.TrimRight(cfg.BaseURL, "/") + "/realms/" + cfg.Realm
	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{issuer + "/protocol/openid-connect/certs"})
	if err != nil {
		return nil, fmt.Errorf("load keycloak jwks: %w", err)
	}
	return &Provider{cfg: cfg, issuer: issuer, jwks: jwks, http: &http.Client{Timeout: 5 * time.Second}}, nil
}

func (p *Provider) ValidateToken(ctx context.Context, token string) (identity.Claims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, p.jwks.Keyfunc,
		jwt.WithIssuer(p.issuer),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return identity.Claims{}, fmt.Errorf("validate token: %w", err)
	}
	if p.cfg.Audience != "" && !audienceMatches(claims, p.cfg.Audience) {
		return identity.Claims{}, fmt.Errorf("validate token: audience mismatch")
	}

	sub, _ := claims["sub"].(string)
	username, _ := claims["preferred_username"].(string)
	email, _ := claims["email"].(string)
	return identity.Claims{Subject: sub, Username: username, Email: email}, nil
}

// audienceMatches accepts either an explicit `aud` claim containing the
// configured audience, or - since Keycloak public clients typically don't
// populate `aud` at all - an `azp` (authorized party) claim matching it.
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

func (p *Provider) CreateIdentity(ctx context.Context, in identity.CreateIdentityInput) (identity.ProviderIdentity, error) {
	adminToken, err := p.adminToken(ctx)
	if err != nil {
		return identity.ProviderIdentity{}, err
	}

	body, err := json.Marshal(map[string]any{
		"username": in.Username,
		"email":    in.Email,
		"enabled":  true,
		"credentials": []map[string]any{
			{"type": "password", "value": in.Password, "temporary": false},
		},
	})
	if err != nil {
		return identity.ProviderIdentity{}, fmt.Errorf("marshal create user body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.adminURL("/users"), strings.NewReader(string(body)))
	if err != nil {
		return identity.ProviderIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return identity.ProviderIdentity{}, fmt.Errorf("create keycloak user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return identity.ProviderIdentity{}, fmt.Errorf("create keycloak user: unexpected status %d: %s", resp.StatusCode, readBody(resp))
	}

	// Keycloak returns the new user's ID in the Location header, not the body.
	loc := resp.Header.Get("Location")
	idx := strings.LastIndex(loc, "/")
	if idx == -1 || idx == len(loc)-1 {
		return identity.ProviderIdentity{}, fmt.Errorf("create keycloak user: missing id in Location header %q", loc)
	}
	return p.GetIdentity(ctx, loc[idx+1:])
}

func (p *Provider) GetIdentity(ctx context.Context, providerSubject string) (identity.ProviderIdentity, error) {
	adminToken, err := p.adminToken(ctx)
	if err != nil {
		return identity.ProviderIdentity{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.adminURL("/users/"+url.PathEscape(providerSubject)), nil)
	if err != nil {
		return identity.ProviderIdentity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return identity.ProviderIdentity{}, fmt.Errorf("get keycloak user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return identity.ProviderIdentity{}, identity.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return identity.ProviderIdentity{}, fmt.Errorf("get keycloak user: unexpected status %d: %s", resp.StatusCode, readBody(resp))
	}

	var raw struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return identity.ProviderIdentity{}, fmt.Errorf("decode keycloak user: %w", err)
	}
	return identity.ProviderIdentity{
		ProviderSubject: raw.ID, Username: raw.Username, Email: raw.Email, Enabled: raw.Enabled,
	}, nil
}

func (p *Provider) DisableIdentity(ctx context.Context, providerSubject string) error {
	adminToken, err := p.adminToken(ctx)
	if err != nil {
		return err
	}
	body := strings.NewReader(`{"enabled":false}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.adminURL("/users/"+url.PathEscape(providerSubject)), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("disable keycloak user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return identity.ErrNotFound
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("disable keycloak user: unexpected status %d: %s", resp.StatusCode, readBody(resp))
	}
	return nil
}

// adminToken fetches a fresh admin access token per call. Admin operations
// are low-frequency (identity provisioning, not the request hot path), so
// the simplicity of not caching/refreshing a token outweighs the extra
// round trip.
func (p *Provider) adminToken(ctx context.Context) (string, error) {
	form := url.Values{
		"client_id":  {"admin-cli"},
		"grant_type": {"password"},
		"username":   {p.cfg.AdminUsername},
		"password":   {p.cfg.AdminPassword},
	}
	tokenURL := strings.TrimRight(p.cfg.BaseURL, "/") + "/realms/master/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch keycloak admin token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch keycloak admin token: unexpected status %d: %s", resp.StatusCode, readBody(resp))
	}

	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode keycloak admin token: %w", err)
	}
	return body.AccessToken, nil
}

func (p *Provider) adminURL(path string) string {
	return strings.TrimRight(p.cfg.BaseURL, "/") + "/admin/realms/" + p.cfg.Realm + path
}

func readBody(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return string(b)
}
