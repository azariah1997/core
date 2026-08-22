package pulseprefs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// QuietHoursFactory and MutesFactory build Core adapters bound to one
// caller's authenticated client - injected from the composition root
// (internal/api/router.go), same reason every other module's *Factory
// type exists for.
type QuietHoursFactory func(client *coresdk.Client) CoreQuietHours
type MutesFactory func(client *coresdk.Client) CoreMutes

func RegisterRoutes(mux *http.ServeMux, svc *Service, newQuietHours QuietHoursFactory, newMutes MutesFactory, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/pulse/preferences", requireUser(getPreferencesHandler(svc)))
	mux.Handle("PUT /v1/pulse/preferences", requireUser(setPreferencesHandler(svc)))

	mux.Handle("GET /v1/pulse/preferences/quiet-hours", requireUser(getQuietHoursHandler(svc, newQuietHours)))
	mux.Handle("PUT /v1/pulse/preferences/quiet-hours", requireUser(setQuietHoursHandler(svc, newQuietHours)))

	mux.Handle("POST /v1/pulse/preferences/mutes", requireUser(muteHandler(svc, newMutes)))
	mux.Handle("GET /v1/pulse/preferences/mutes", requireUser(listMutesHandler(svc, newMutes)))
	mux.Handle("DELETE /v1/pulse/preferences/mutes/{userId}", requireUser(unmuteHandler(svc, newMutes)))
}

func caller(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := pulseauth.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
		return "", false
	}
	return id, true
}

func coreClient(w http.ResponseWriter, r *http.Request) (*coresdk.Client, bool) {
	client, ok := pulseauth.ClientFromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "core client missing from context"))
		return nil, false
	}
	return client, true
}

type preferencesResponse struct {
	NotificationDetail string  `json:"notificationDetail"`
	HapticIntensity    float64 `json:"hapticIntensity"`
}

func toPreferencesResponse(p Preferences) preferencesResponse {
	return preferencesResponse{NotificationDetail: string(p.NotificationDetail), HapticIntensity: p.HapticIntensity}
}

func getPreferencesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := caller(w, r)
		if !ok {
			return
		}
		p, err := svc.Get(r.Context(), callerID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toPreferencesResponse(p))
	}
}

func setPreferencesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := caller(w, r)
		if !ok {
			return
		}
		var body struct {
			NotificationDetail string  `json:"notificationDetail"`
			HapticIntensity    float64 `json:"hapticIntensity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		p, err := svc.Set(r.Context(), callerID, SetPreferencesInput{
			NotificationDetail: NotificationDetail(body.NotificationDetail),
			HapticIntensity:    body.HapticIntensity,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toPreferencesResponse(p))
	}
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

func getQuietHoursHandler(svc *Service, newQuietHours QuietHoursFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ok := coreClient(w, r)
		if !ok {
			return
		}
		q, err := svc.GetQuietHours(r.Context(), newQuietHours(client))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toQuietHoursResponse(q))
	}
}

func setQuietHoursHandler(svc *Service, newQuietHours QuietHoursFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ok := coreClient(w, r)
		if !ok {
			return
		}
		var body struct {
			Timezone    string `json:"timezone"`
			StartMinute int    `json:"startMinute"`
			EndMinute   int    `json:"endMinute"`
			Enabled     bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		q, err := svc.SetQuietHours(r.Context(), newQuietHours(client), SetQuietHoursInput{
			Timezone: body.Timezone, StartMinute: body.StartMinute, EndMinute: body.EndMinute, Enabled: body.Enabled,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toQuietHoursResponse(q))
	}
}

type muteResponse struct {
	ID          string `json:"id"`
	MutedUserID string `json:"mutedUserId"`
	CreatedAt   string `json:"createdAt"`
}

func toMuteResponse(m Mute) muteResponse {
	return muteResponse{ID: m.ID, MutedUserID: m.MutedUserID, CreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")}
}

func muteHandler(svc *Service, newMutes MutesFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ok := coreClient(w, r)
		if !ok {
			return
		}
		var body struct {
			MutedUserID string `json:"mutedUserId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		m, err := svc.Mute(r.Context(), newMutes(client), body.MutedUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toMuteResponse(m))
	}
}

func listMutesHandler(svc *Service, newMutes MutesFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ok := coreClient(w, r)
		if !ok {
			return
		}
		list, err := svc.ListMutes(r.Context(), newMutes(client))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]muteResponse, 0, len(list))
		for _, m := range list {
			items = append(items, toMuteResponse(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func unmuteHandler(svc *Service, newMutes MutesFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, ok := coreClient(w, r)
		if !ok {
			return
		}
		if err := svc.Unmute(r.Context(), newMutes(client), r.PathValue("userId")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, verr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "preferences not found"))
	default:
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "unexpected error"))
	}
}
