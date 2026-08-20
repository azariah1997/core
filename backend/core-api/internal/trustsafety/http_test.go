package trustsafety_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/trustsafety"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestMuteHandlerRejectsMutingSelf(t *testing.T) {
	svc := newService(fakeModerator{}, true)
	mux := http.NewServeMux()
	trustsafety.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/trustsafety/mutes", strings.NewReader(`{"mutedUserId":"u1"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMuteAndListRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService(fakeModerator{}, true)
	mux := http.NewServeMux()
	trustsafety.RegisterRoutes(mux, svc, fixedUser("u1"))

	muteReq := httptest.NewRequest("POST", "/v1/trustsafety/mutes", strings.NewReader(`{"mutedUserId":"u2"}`))
	muteRR := httptest.NewRecorder()
	mux.ServeHTTP(muteRR, muteReq)
	if muteRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", muteRR.Code, muteRR.Body.String())
	}

	listReq := httptest.NewRequest("GET", "/v1/trustsafety/mutes", nil)
	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), "u2") {
		t.Fatalf("expected the muted user in the response, got %s", listRR.Body.String())
	}
}

func TestListCasesHandlerRequiresModerator(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	mux := http.NewServeMux()
	trustsafety.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("GET", "/v1/trustsafety/cases", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestReportBanAppealAndReviewRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)

	modMux := http.NewServeMux()
	trustsafety.RegisterRoutes(modMux, svc, fixedUser("mod"))

	banReq := httptest.NewRequest("POST", "/v1/trustsafety/bans", strings.NewReader(`{"userId":"target","reason":"abuse"}`))
	banRR := httptest.NewRecorder()
	modMux.ServeHTTP(banRR, banReq)
	if banRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", banRR.Code, banRR.Body.String())
	}
	var ban struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(banRR.Body.Bytes(), &ban); err != nil {
		t.Fatalf("decode ban: %v", err)
	}

	targetMux := http.NewServeMux()
	trustsafety.RegisterRoutes(targetMux, svc, fixedUser("target"))
	appealReq := httptest.NewRequest("POST", "/v1/trustsafety/appeals",
		strings.NewReader(`{"targetType":"ban","targetId":"`+ban.ID+`","reason":"mistake"}`))
	appealRR := httptest.NewRecorder()
	targetMux.ServeHTTP(appealRR, appealReq)
	if appealRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", appealRR.Code, appealRR.Body.String())
	}
	var appeal struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(appealRR.Body.Bytes(), &appeal); err != nil {
		t.Fatalf("decode appeal: %v", err)
	}

	reviewReq := httptest.NewRequest("POST", "/v1/trustsafety/appeals/"+appeal.ID+"/review", strings.NewReader(`{"approve":true}`))
	reviewRR := httptest.NewRecorder()
	modMux.ServeHTTP(reviewRR, reviewReq)
	if reviewRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", reviewRR.Code, reviewRR.Body.String())
	}
	if !strings.Contains(reviewRR.Body.String(), `"status":"approved"`) {
		t.Fatalf("expected the appeal to be approved, got %s", reviewRR.Body.String())
	}
}
