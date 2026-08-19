package coresdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPasswordTokenSourceMintsAndCachesToken(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "password" || r.Form.Get("username") != "demo" {
			t.Errorf("unexpected token request form: %v", r.Form)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "minted-token",
			"expires_in":   300,
		})
	}))
	defer srv.Close()

	ts := &PasswordTokenSource{
		KeycloakURL: srv.URL,
		Realm:       "core",
		ClientID:    "core-platform",
		Username:    "demo",
		Password:    "demo",
	}

	tok, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token returned error: %v", err)
	}
	if tok != "minted-token" {
		t.Fatalf("expected minted-token, got %q", tok)
	}

	// Second call within the token's lifetime should be served from
	// cache, not mint a new token.
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatalf("Token returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 token request (second call should hit cache), got %d", calls)
	}
}

func TestPasswordTokenSourceRefreshesNearExpiry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-call-" + time.Now().String(),
			"expires_in":   1, // 1 second - guaranteed to be inside the refresh skew
		})
	}))
	defer srv.Close()

	ts := &PasswordTokenSource{
		KeycloakURL: srv.URL,
		Realm:       "core",
		ClientID:    "core-platform",
		Username:    "demo",
		Password:    "demo",
		refreshSkew: 5 * time.Second,
	}

	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatalf("Token returned error: %v", err)
	}
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatalf("Token returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected the second call to trigger a real refresh (token expires inside the skew window), got %d calls", calls)
	}
}

func TestStaticTokenSourceNeverCallsOut(t *testing.T) {
	tok, err := StaticTokenSource("fixed").Token(context.Background())
	if err != nil || tok != "fixed" {
		t.Fatalf("expected fixed/nil, got %q/%v", tok, err)
	}
}
