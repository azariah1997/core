package pulseinteractions

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// CoreFactory builds a CoreRelationships bound to one caller's
// authenticated client - injected from the composition root, same
// import-cycle reason as pulse-connections/bond's own http.go.
type CoreFactory func(client *coresdk.Client) CoreRelationships

func RegisterRoutes(mux *http.ServeMux, svc *Service, newCore CoreFactory, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/pulse/interactions", requireUser(createHandler(svc, newCore)))
	mux.Handle("POST /v1/pulse/interactions/{id}/start", requireUser(startHandler(svc)))
	mux.Handle("POST /v1/pulse/interactions/{id}/stop", requireUser(stopHandler(svc)))
	mux.Handle("GET /v1/pulse/interactions/{id}", requireUser(getHandler(svc)))
	mux.Handle("GET /v1/pulse/interactions", requireUser(listMineHandler(svc)))
}

type interactionResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	OtherUserID  string  `json:"otherUserId"`
	Role         string  `json:"role"` // "sender" or "receiver", from the caller's perspective
	Status       string  `json:"status"`
	DeliveryMode string  `json:"deliveryMode"`
	StartedAt    *string `json:"startedAt,omitempty"`
	EndedAt      *string `json:"endedAt,omitempty"`
	DurationMs   *int    `json:"durationMs,omitempty"`
	CreatedAt    string  `json:"createdAt"`
}

func toInteractionResponse(callerID string, i Interaction) interactionResponse {
	role := "sender"
	if i.SenderID != callerID {
		role = "receiver"
	}
	resp := interactionResponse{
		ID: i.ID, Type: string(i.Type), OtherUserID: i.otherUser(callerID), Role: role,
		Status: string(i.Status), DeliveryMode: string(i.DeliveryMode), DurationMs: i.DurationMs,
		CreatedAt: i.CreatedAt.UTC().Format(timeFormat),
	}
	if i.StartedAt != nil {
		s := i.StartedAt.UTC().Format(timeFormat)
		resp.StartedAt = &s
	}
	if i.EndedAt != nil {
		s := i.EndedAt.UTC().Format(timeFormat)
		resp.EndedAt = &s
	}
	return resp
}

func createHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		client, ok := pulseauth.ClientFromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "core client missing from context"))
			return
		}
		var body struct {
			Type            string `json:"type"`
			ReceiverID      string `json:"receiverId"`
			ClientRequestID string `json:"clientRequestId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		i, err := svc.Create(r.Context(), newCore(client), callerID, CreateInput{ReceiverID: body.ReceiverID, ClientRequestID: body.ClientRequestID})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toInteractionResponse(callerID, i))
	}
}

func startHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		i, err := svc.Start(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toInteractionResponse(callerID, i))
	}
}

func stopHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		i, err := svc.Stop(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toInteractionResponse(callerID, i))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		i, err := svc.Get(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toInteractionResponse(callerID, i))
	}
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		list, err := svc.ListMine(r.Context(), callerID, limit)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]interactionResponse, 0, len(list))
		for _, i := range list {
			items = append(items, toInteractionResponse(callerID, i))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	var apiErr *coresdk.APIError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "interaction not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrNotConnected):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrBlocked):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrInvalidTransition):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrRateLimited):
		apperr.Write(w, r, apperr.New(apperr.CodeRateLimited, err.Error()))
	case errors.As(err, &apiErr):
		apperr.Write(w, r, apperr.New(apperr.Code(apiErr.Code), apiErr.Message))
	default:
		slog.Default().Error("unhandled pulseinteractions error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
