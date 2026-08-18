package files

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

// RegisterRoutes wires the file endpoints, all requiring an authenticated
// caller. Ownership/visibility/admin checks happen inside Service, not
// here, the same split every other module in this repo uses.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/files", requireUser(requestUploadHandler(svc)))
	mux.Handle("POST /v1/files/purge-expired", requireUser(purgeExpiredHandler(svc)))
	mux.Handle("POST /v1/files/{id}/confirm", requireUser(confirmUploadHandler(svc)))
	mux.Handle("GET /v1/files/{id}", requireUser(getHandler(svc)))
	mux.Handle("GET /v1/files/{id}/download", requireUser(getDownloadURLHandler(svc)))
	mux.Handle("GET /v1/files", requireUser(listMineHandler(svc)))
	mux.Handle("DELETE /v1/files/{id}", requireUser(deleteHandler(svc)))
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

type fileResponse struct {
	ID          string     `json:"id"`
	AppID       string     `json:"appId"`
	OwnerUserID string     `json:"ownerUserId"`
	FileName    string     `json:"fileName"`
	MimeType    string     `json:"mimeType"`
	ByteSize    int64      `json:"byteSize"`
	Checksum    string     `json:"checksum,omitempty"`
	Visibility  Visibility `json:"visibility"`
	Status      Status     `json:"status"`
	ExpiresAt   *string    `json:"expiresAt,omitempty"`
	CreatedAt   string     `json:"createdAt"`
	UpdatedAt   string     `json:"updatedAt"`
}

func toFileResponse(f File) fileResponse {
	resp := fileResponse{
		ID: f.ID, AppID: f.AppID, OwnerUserID: f.OwnerUserID, FileName: f.FileName,
		MimeType: f.MimeType, ByteSize: f.ByteSize, Checksum: f.Checksum,
		Visibility: f.Visibility, Status: f.Status,
		CreatedAt: f.CreatedAt.UTC().Format(timeFormat), UpdatedAt: f.UpdatedAt.UTC().Format(timeFormat),
	}
	if f.ExpiresAt != nil {
		s := f.ExpiresAt.UTC().Format(timeFormat)
		resp.ExpiresAt = &s
	}
	return resp
}

func requestUploadHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID         string     `json:"appId"`
			FileName      string     `json:"fileName"`
			MimeType      string     `json:"mimeType"`
			SizeBytes     int64      `json:"sizeBytes"`
			Checksum      string     `json:"checksum"`
			Visibility    Visibility `json:"visibility"`
			RetentionDays *int       `json:"retentionDays"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		f, uploadURL, expiresAt, err := svc.RequestUpload(r.Context(), caller, RequestUploadInput{
			AppID: body.AppID, FileName: body.FileName, MimeType: body.MimeType, SizeBytes: body.SizeBytes,
			Checksum: body.Checksum, Visibility: body.Visibility, RetentionDays: body.RetentionDays,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]any{
			"file": toFileResponse(f), "uploadUrl": uploadURL, "uploadUrlExpiresAt": expiresAt.UTC().Format(timeFormat),
		})
	}
}

func confirmUploadHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		f, err := svc.ConfirmUpload(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toFileResponse(f))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		f, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toFileResponse(f))
	}
}

func getDownloadURLHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		url, expiresAt, err := svc.GetDownloadURL(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"downloadUrl": url, "expiresAt": expiresAt.UTC().Format(timeFormat)})
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
		items := make([]fileResponse, 0, len(result.Items))
		for _, f := range result.Items {
			items = append(items, toFileResponse(f))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
	}
}

func deleteHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if err := svc.Delete(r.Context(), caller, r.PathValue("id")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func purgeExpiredHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		count, err := svc.PurgeExpired(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"purgedCount": count})
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
	case errors.Is(err, ErrMimeTypeNotAllowed), errors.Is(err, ErrSizeLimitExceeded), errors.Is(err, ErrChecksumMismatch), errors.Is(err, ErrNotPending), errors.Is(err, ErrNotActive):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, err.Error()))
	default:
		slog.Default().Error("unhandled files error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
