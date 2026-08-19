package coresdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenSource returns a valid Bearer token for the current request,
// refreshing it internally if needed. Every core-api call goes through
// one - "authentication" and "token refresh" are the SDK's own job, not
// something every calling product should reimplement.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource wraps an already-minted token (e.g. one a caller
// obtained some other way) with no refresh behavior - useful for
// short-lived scripts and tests.
type StaticTokenSource string

func (s StaticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

// PasswordTokenSource performs a real Resource Owner Password
// Credentials grant against Keycloak (the same grant apps/admin's
// server-side login and every phase's live-validation curl sequence in
// this repo has used) and transparently refreshes the token before it
// expires - a caller just asks for Token(ctx) and either gets the
// cached one or a freshly refreshed one, never an expired one.
type PasswordTokenSource struct {
	KeycloakURL string
	Realm       string
	ClientID    string
	Username    string
	Password    string
	HTTPClient  *http.Client

	// refreshSkew is how long before real expiry a token is treated as
	// already-expired, so a request never races a token dying mid-flight.
	refreshSkew time.Duration

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (p *PasswordTokenSource) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	skew := p.refreshSkew
	if skew == 0 {
		skew = 15 * time.Second
	}
	if p.token != "" && time.Now().Add(skew).Before(p.expiresAt) {
		return p.token, nil
	}

	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {p.ClientID},
		"username":   {p.Username},
		"password":   {p.Password},
	}
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", strings.TrimRight(p.KeycloakURL, "/"), p.Realm)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("coresdk: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("coresdk: token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("coresdk: read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("coresdk: token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("coresdk: decode token response: %w", err)
	}

	p.token = payload.AccessToken
	p.expiresAt = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	return p.token, nil
}
