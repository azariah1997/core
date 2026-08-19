package analytics

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the admin-only debug listing behind requireUser.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/analytics/events", requireUser(listRecentHandler(svc)))
}

// RegisterTrackRoute is separate from RegisterRoutes specifically so
// main.go can wire it with no auth middleware at all - see
// analytics/README.md for why an analytics ingestion endpoint, unlike
// every other write in this platform, has to accept pre-login/
// unauthenticated traffic to be useful, and how rate limiting by IP
// (not a Bearer token) is what protects it instead.
func RegisterTrackRoute(mux *http.ServeMux, svc *Service) {
	mux.Handle("POST /v1/analytics/events", trackHandler(svc))
}

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type trackEventBody struct {
	EventName   string         `json:"eventName"`
	UserID      string         `json:"userId"`
	AnonymousID string         `json:"anonymousId"`
	AppID       string         `json:"appId"`
	SessionID   string         `json:"sessionId"`
	Timestamp   *time.Time     `json:"timestamp"`
	Properties  map[string]any `json:"properties"`
	Context     map[string]any `json:"context"`
}

func trackHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Events []trackEventBody `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		inputs := make([]TrackInput, 0, len(body.Events))
		for _, e := range body.Events {
			in := TrackInput{
				EventName: e.EventName, UserID: e.UserID, AnonymousID: e.AnonymousID, AppID: e.AppID,
				SessionID: e.SessionID, Properties: e.Properties, Context: e.Context,
			}
			if e.Timestamp != nil {
				in.OccurredAt = *e.Timestamp
			}
			inputs = append(inputs, in)
		}
		if err := svc.Track(r.Context(), clientIP(r), inputs); err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusAccepted, map[string]any{"accepted": len(inputs)})
	}
}

type eventResponse struct {
	ID          string         `json:"id"`
	EventName   string         `json:"eventName"`
	UserID      string         `json:"userId,omitempty"`
	AnonymousID string         `json:"anonymousId,omitempty"`
	AppID       string         `json:"appId"`
	SessionID   string         `json:"sessionId,omitempty"`
	OccurredAt  string         `json:"occurredAt"`
	Properties  map[string]any `json:"properties,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
	IngestedAt  string         `json:"ingestedAt"`
	Flushed     bool           `json:"flushed"`
	BatchRef    string         `json:"batchRef,omitempty"`
}

func toEventResponse(e Event) eventResponse {
	return eventResponse{
		ID: e.ID, EventName: e.EventName, UserID: e.UserID, AnonymousID: e.AnonymousID, AppID: e.AppID,
		SessionID: e.SessionID, OccurredAt: e.OccurredAt.UTC().Format(timeFormat), Properties: e.Properties,
		Context: e.Context, IngestedAt: e.IngestedAt.UTC().Format(timeFormat), Flushed: e.FlushedAt != nil, BatchRef: e.BatchRef,
	}
}

func listRecentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := users.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		list, err := svc.ListRecent(r.Context(), u.ID, limit)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]eventResponse, 0, len(list))
		for _, e := range list {
			items = append(items, toEventResponse(e))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"items": items,
			"note":  "operational debug view only - not for analytical queries; see analytics/README.md",
		})
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrRateLimited):
		apperr.Write(w, r, apperr.New(apperr.CodeRateLimited, err.Error()))
	default:
		slog.Default().Error("unhandled analytics error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
