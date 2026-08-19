package billing

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the user-facing entitlement/payment endpoints
// (all requiring an authenticated caller, via requireUser) plus the
// webhook receiver (registered separately - see RegisterWebhookRoute -
// deliberately NOT wrapped in requireUser, since a payment provider has
// no platform Bearer token; its identity is the webhook signature
// itself).
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/billing/entitlements", requireUser(listEntitlementsHandler(svc)))
	mux.Handle("GET /v1/billing/entitlements/check/{key}", requireUser(checkEntitlementHandler(svc)))
	mux.Handle("POST /v1/billing/entitlements", requireUser(grantEntitlementHandler(svc)))
	mux.Handle("POST /v1/billing/entitlements/{id}/revoke", requireUser(revokeEntitlementHandler(svc)))
	mux.Handle("GET /v1/billing/payments", requireUser(listPaymentsHandler(svc)))
}

// RegisterWebhookRoute is separate from RegisterRoutes specifically so
// main.go can wire it onto the mux with no auth middleware at all - see
// this package's README for why that's the correct, not a missing,
// authentication story here.
func RegisterWebhookRoute(mux *http.ServeMux, svc *Service) {
	mux.Handle("POST /v1/billing/webhooks/{provider}", webhookHandler(svc))
}

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

func callerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	u, ok := users.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
		return "", false
	}
	return u.ID, true
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(timeFormat)
	return &s
}

type entitlementResponse struct {
	ID        string  `json:"id"`
	Key       string  `json:"key"`
	Source    string  `json:"source"`
	GrantedAt string  `json:"grantedAt"`
	ExpiresAt *string `json:"expiresAt,omitempty"`
	RevokedAt *string `json:"revokedAt,omitempty"`
	Active    bool    `json:"active"`
}

func toEntitlementResponse(e Entitlement) entitlementResponse {
	return entitlementResponse{
		ID: e.ID, Key: e.Key, Source: e.Source, GrantedAt: e.GrantedAt.UTC().Format(timeFormat),
		ExpiresAt: formatTimePtr(e.ExpiresAt), RevokedAt: formatTimePtr(e.RevokedAt), Active: e.IsActive(time.Now().UTC()),
	}
}

func listEntitlementsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		targetUserID := r.URL.Query().Get("userId")
		if targetUserID == "" {
			targetUserID = caller
		}
		list, err := svc.ListEntitlements(r.Context(), caller, targetUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]entitlementResponse, 0, len(list))
		for _, e := range list {
			items = append(items, toEntitlementResponse(e))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

// checkEntitlementHandler is the literal "does the calling user have
// entitlement X" endpoint the roadmap describes - self only, since a
// client app checks its own logged-in user's access, not anyone else's
// (that's what listEntitlementsHandler's ?userId= + admin path is for).
func checkEntitlementHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		key := r.PathValue("key")
		active, err := svc.HasEntitlement(r.Context(), caller, key)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"key": key, "active": active})
	}
}

func grantEntitlementHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			UserID    string     `json:"userId"`
			Key       string     `json:"key"`
			ExpiresAt *time.Time `json:"expiresAt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		e, err := svc.GrantEntitlement(r.Context(), caller, GrantEntitlementInput{UserID: body.UserID, Key: body.Key, ExpiresAt: body.ExpiresAt})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toEntitlementResponse(e))
	}
}

func revokeEntitlementHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		e, err := svc.RevokeEntitlement(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toEntitlementResponse(e))
	}
}

type paymentResponse struct {
	ID          string         `json:"id"`
	Provider    string         `json:"provider"`
	ProviderRef string         `json:"providerRef"`
	AmountCents int64          `json:"amountCents"`
	Currency    string         `json:"currency"`
	Status      string         `json:"status"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
}

func toPaymentResponse(p Payment) paymentResponse {
	return paymentResponse{
		ID: p.ID, Provider: p.Provider, ProviderRef: p.ProviderRef, AmountCents: p.AmountCents, Currency: p.Currency,
		Status: string(p.Status), Metadata: p.Metadata, CreatedAt: p.CreatedAt.UTC().Format(timeFormat), UpdatedAt: p.UpdatedAt.UTC().Format(timeFormat),
	}
}

func listPaymentsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		targetUserID := r.URL.Query().Get("userId")
		if targetUserID == "" {
			targetUserID = caller
		}
		list, err := svc.ListPayments(r.Context(), caller, targetUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]paymentResponse, 0, len(list))
		for _, p := range list {
			items = append(items, toPaymentResponse(p))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

// webhookHandler reads the raw request body - never decodes JSON before
// verification - because signature verification must run over the
// exact bytes the provider signed; decoding and re-marshaling first
// (even with identical field values) is not guaranteed to reproduce the
// same bytes.
func webhookHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "could not read request body"))
			return
		}
		provider := r.PathValue("provider")
		sig := r.Header.Get("Stripe-Signature") // the only provider this phase ships; see README for others
		if err := svc.HandleWebhook(r.Context(), provider, payload, sig); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrUnknownProvider), errors.Is(err, ErrUnhandledEvent):
		// A 2xx here would tell Stripe to stop retrying an event type
		// this platform doesn't (yet) act on - NOT_IMPLEMENTED specifically
		// so this platform's own logs and any provider dashboard both
		// show it as "received but not acted on," not silently dropped.
		apperr.Write(w, r, apperr.New(apperr.CodeNotImplemented, err.Error()))
	case errors.Is(err, ErrInvalidSignature), errors.Is(err, ErrSignatureExpired):
		apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, err.Error()))
	default:
		slog.Default().Error("unhandled billing error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
