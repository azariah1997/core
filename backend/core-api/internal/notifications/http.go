package notifications

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the notification endpoints, all requiring an
// authenticated caller. Send/Get/ListDeliveries/Retry are self-or-
// platform.admin (enforced in Service); template management is
// platform.admin only.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/notifications", requireUser(sendHandler(svc)))
	mux.Handle("GET /v1/notifications", requireUser(listMineHandler(svc)))
	mux.Handle("GET /v1/notifications/{id}", requireUser(getHandler(svc)))
	mux.Handle("GET /v1/notifications/{id}/deliveries", requireUser(listDeliveriesHandler(svc)))
	mux.Handle("POST /v1/notifications/{id}/deliveries/{deliveryId}/retry", requireUser(retryDeliveryHandler(svc)))

	mux.Handle("GET /v1/notification-preferences", requireUser(getPreferencesHandler(svc)))
	mux.Handle("PUT /v1/notification-preferences", requireUser(setPreferenceHandler(svc)))
	mux.Handle("GET /v1/notification-preferences/quiet-hours", requireUser(getQuietHoursHandler(svc)))
	mux.Handle("PUT /v1/notification-preferences/quiet-hours", requireUser(setQuietHoursHandler(svc)))

	mux.Handle("POST /v1/notification-templates", requireUser(createTemplateHandler(svc)))
	mux.Handle("GET /v1/notification-templates/{appId}/{key}", requireUser(getTemplateHandler(svc)))
	mux.Handle("PATCH /v1/notification-templates/{appId}/{key}", requireUser(updateTemplateHandler(svc)))
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

type notificationResponse struct {
	ID           string         `json:"id"`
	AppID        string         `json:"appId"`
	UserID       string         `json:"userId"`
	Category     string         `json:"category"`
	Title        string         `json:"title"`
	Body         string         `json:"body"`
	Data         map[string]any `json:"data,omitempty"`
	Channels     []Channel      `json:"channels"`
	SentByUserID string         `json:"sentByUserId,omitempty"`
	CreatedAt    string         `json:"createdAt"`
}

func toNotificationResponse(n Notification) notificationResponse {
	return notificationResponse{
		ID: n.ID, AppID: n.AppID, UserID: n.UserID, Category: n.Category,
		Title: n.Title, Body: n.Body, Data: n.Data, Channels: n.Channels, SentByUserID: n.SentByUserID,
		CreatedAt: n.CreatedAt.UTC().Format(timeFormat),
	}
}

type deliveryResponse struct {
	ID             string  `json:"id"`
	NotificationID string  `json:"notificationId"`
	Channel        Channel `json:"channel"`
	Status         Status  `json:"status"`
	ProviderRef    string  `json:"providerRef,omitempty"`
	Error          string  `json:"error,omitempty"`
	Attempts       int     `json:"attempts"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	DeliveredAt    *string `json:"deliveredAt,omitempty"`
}

func toDeliveryResponse(d NotificationDelivery) deliveryResponse {
	resp := deliveryResponse{
		ID: d.ID, NotificationID: d.NotificationID, Channel: d.Channel, Status: d.Status,
		ProviderRef: d.ProviderRef, Error: d.Error, Attempts: d.Attempts,
		CreatedAt: d.CreatedAt.UTC().Format(timeFormat), UpdatedAt: d.UpdatedAt.UTC().Format(timeFormat),
	}
	if d.DeliveredAt != nil {
		s := d.DeliveredAt.UTC().Format(timeFormat)
		resp.DeliveredAt = &s
	}
	return resp
}

type templateResponse struct {
	ID            string `json:"id"`
	AppID         string `json:"appId"`
	Key           string `json:"key"`
	TitleTemplate string `json:"titleTemplate"`
	BodyTemplate  string `json:"bodyTemplate"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func toTemplateResponse(t NotificationTemplate) templateResponse {
	return templateResponse{
		ID: t.ID, AppID: t.AppID, Key: t.Key, TitleTemplate: t.TitleTemplate, BodyTemplate: t.BodyTemplate,
		CreatedAt: t.CreatedAt.UTC().Format(timeFormat), UpdatedAt: t.UpdatedAt.UTC().Format(timeFormat),
	}
}

type preferenceResponse struct {
	Category string  `json:"category"`
	Channel  Channel `json:"channel"`
	Enabled  bool    `json:"enabled"`
}

func toPreferenceResponse(p NotificationPreference) preferenceResponse {
	return preferenceResponse{Category: p.Category, Channel: p.Channel, Enabled: p.Enabled}
}

type quietHoursResponse struct {
	Timezone    string `json:"timezone"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	Enabled     bool   `json:"enabled"`
}

func toQuietHoursResponse(q QuietHours) quietHoursResponse {
	return quietHoursResponse{Timezone: q.Timezone, StartMinute: q.StartMinute, EndMinute: q.EndMinute, Enabled: q.Enabled}
}

func sendHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID        string         `json:"appId"`
			UserID       string         `json:"userId"`
			Category     string         `json:"category"`
			Channels     []Channel      `json:"channels"`
			TemplateKey  string         `json:"templateKey"`
			TemplateData map[string]any `json:"templateData"`
			Title        string         `json:"title"`
			Body         string         `json:"body"`
			Data         map[string]any `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		n, deliveries, err := svc.Send(r.Context(), caller, SendInput{
			AppID: body.AppID, UserID: body.UserID, Category: body.Category, Channels: body.Channels,
			TemplateKey: body.TemplateKey, TemplateData: body.TemplateData, Title: body.Title, Body: body.Body, Data: body.Data,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		deliveryItems := make([]deliveryResponse, 0, len(deliveries))
		for _, d := range deliveries {
			deliveryItems = append(deliveryItems, toDeliveryResponse(d))
		}
		resp := toNotificationResponse(n)
		httpx.JSON(w, http.StatusCreated, map[string]any{"notification": resp, "deliveries": deliveryItems})
	}
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		result, err := svc.ListMine(r.Context(), caller, ListParams{Limit: limit, Cursor: r.URL.Query().Get("cursor")})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]notificationResponse, 0, len(result.Items))
		for _, n := range result.Items {
			items = append(items, toNotificationResponse(n))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		n, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toNotificationResponse(n))
	}
}

func listDeliveriesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListDeliveries(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]deliveryResponse, 0, len(list))
		for _, d := range list {
			items = append(items, toDeliveryResponse(d))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func retryDeliveryHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		d, err := svc.RetryDelivery(r.Context(), caller, r.PathValue("id"), r.PathValue("deliveryId"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toDeliveryResponse(d))
	}
}

func getPreferencesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		appID := r.URL.Query().Get("appId")
		if appID == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "appId query parameter is required"))
			return
		}
		list, err := svc.GetPreferences(r.Context(), caller, appID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]preferenceResponse, 0, len(list))
		for _, p := range list {
			items = append(items, toPreferenceResponse(p))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func setPreferenceHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID    string  `json:"appId"`
			Category string  `json:"category"`
			Channel  Channel `json:"channel"`
			Enabled  bool    `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		if body.AppID == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "appId is required"))
			return
		}
		p, err := svc.SetPreference(r.Context(), caller, body.AppID, SetPreferenceInput{Category: body.Category, Channel: body.Channel, Enabled: body.Enabled})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toPreferenceResponse(p))
	}
}

func getQuietHoursHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		appID := r.URL.Query().Get("appId")
		if appID == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "appId query parameter is required"))
			return
		}
		q, err := svc.GetQuietHours(r.Context(), caller, appID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toQuietHoursResponse(q))
	}
}

func setQuietHoursHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID       string `json:"appId"`
			Timezone    string `json:"timezone"`
			StartMinute int    `json:"startMinute"`
			EndMinute   int    `json:"endMinute"`
			Enabled     bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		if body.AppID == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "appId is required"))
			return
		}
		q, err := svc.SetQuietHours(r.Context(), caller, body.AppID, SetQuietHoursInput{
			Timezone: body.Timezone, StartMinute: body.StartMinute, EndMinute: body.EndMinute, Enabled: body.Enabled,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toQuietHoursResponse(q))
	}
}

func createTemplateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID         string `json:"appId"`
			Key           string `json:"key"`
			TitleTemplate string `json:"titleTemplate"`
			BodyTemplate  string `json:"bodyTemplate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		t, err := svc.CreateTemplate(r.Context(), caller, CreateTemplateInput{
			AppID: body.AppID, Key: body.Key, TitleTemplate: body.TitleTemplate, BodyTemplate: body.BodyTemplate,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toTemplateResponse(t))
	}
}

func getTemplateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		t, err := svc.GetTemplate(r.Context(), r.PathValue("appId"), r.PathValue("key"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toTemplateResponse(t))
	}
}

func updateTemplateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			TitleTemplate *string `json:"titleTemplate"`
			BodyTemplate  *string `json:"bodyTemplate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		t, err := svc.UpdateTemplate(r.Context(), caller, r.PathValue("appId"), r.PathValue("key"),
			UpdateTemplateInput{TitleTemplate: body.TitleTemplate, BodyTemplate: body.BodyTemplate})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toTemplateResponse(t))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrDeliveryNotFound), errors.Is(err, ErrTemplateNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrTemplateKeyTaken):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled notifications error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
