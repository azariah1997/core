package pulseconnections

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// CoreFactory builds a CoreRelationships bound to one caller's
// authenticated client. Passed in from the composition root
// (internal/api/router.go), which is the only place allowed to know
// about the concrete internal/pulseconnections/core adapter - this
// package (and its own tests) only ever depends on the CoreRelationships
// interface, never the adapter, avoiding an import cycle (the adapter
// itself imports this package's domain types) and matching Core's own
// convention of wiring concrete storage/provider implementations at the
// composition root, never inside a domain module's own http.go.
type CoreFactory func(client *coresdk.Client) CoreRelationships

// GroupsFactory builds a CoreGroups bound to one caller's authenticated
// client - same import-cycle-avoidance reason as CoreFactory.
type GroupsFactory func(client *coresdk.Client) CoreGroups

// RegisterRoutes wires the connection-experience endpoints. Every
// handler builds a CoreRelationships from the caller's own
// authenticated client (pulseauth.ClientFromContext) - never a fixed
// service-level credential - so every Core relationships call is made
// as the real caller and gets Core's own authorization applied.
func RegisterRoutes(mux *http.ServeMux, svc *Service, newCore CoreFactory, newGroups GroupsFactory, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/pulse/connections/requests", requireUser(requestHandler(svc, newCore)))
	mux.Handle("POST /v1/pulse/connections/requests/{id}/accept", requireUser(acceptHandler(svc, newCore)))
	mux.Handle("POST /v1/pulse/connections/requests/{id}/decline", requireUser(declineHandler(svc, newCore)))
	mux.Handle("DELETE /v1/pulse/connections/{id}", requireUser(removeHandler(svc, newCore)))
	mux.Handle("GET /v1/pulse/connections", requireUser(listMineHandler(svc, newCore)))
	mux.Handle("PATCH /v1/pulse/connections/{id}", requireUser(setClassificationHandler(svc)))

	mux.Handle("POST /v1/pulse/circles", requireUser(createCircleHandler(svc, newGroups)))
	mux.Handle("GET /v1/pulse/circles", requireUser(listCirclesHandler(svc, newGroups)))
	mux.Handle("GET /v1/pulse/circles/{id}/members", requireUser(listCircleMembersHandler(svc, newGroups)))
	mux.Handle("POST /v1/pulse/circles/{id}/members", requireUser(addCircleMemberHandler(svc, newGroups, newCore)))
	mux.Handle("DELETE /v1/pulse/circles/{id}/members/{userId}", requireUser(removeCircleMemberHandler(svc, newGroups)))
}

func callerAndCore(w http.ResponseWriter, r *http.Request, newCore CoreFactory) (string, CoreRelationships, bool) {
	callerID, ok := pulseauth.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
		return "", nil, false
	}
	client, ok := pulseauth.ClientFromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "core client missing from context"))
		return "", nil, false
	}
	return callerID, newCore(client), true
}

type connectionResponse struct {
	RelationshipID string `json:"relationshipId"`
	OtherUserID    string `json:"otherUserId"`
	Status         string `json:"status"`
	Direction      string `json:"direction"`
	Classification string `json:"classification"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func toConnectionResponse(c Connection) connectionResponse {
	return connectionResponse{
		RelationshipID: c.RelationshipID, OtherUserID: c.OtherUserID, Status: c.Status, Direction: c.Direction,
		Classification: string(c.Classification),
		CreatedAt:      c.CreatedAt.UTC().Format(timeFormat), UpdatedAt: c.UpdatedAt.UTC().Format(timeFormat),
	}
}

func requestHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		var body struct {
			TargetUserID string `json:"targetUserId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		conn, err := svc.RequestConnection(r.Context(), core, callerID, RequestInput{TargetUserID: body.TargetUserID})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toConnectionResponse(conn))
	}
}

func acceptHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		conn, err := svc.Accept(r.Context(), core, callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toConnectionResponse(conn))
	}
}

func declineHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		conn, err := svc.Decline(r.Context(), core, callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toConnectionResponse(conn))
	}
}

func removeHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		if err := svc.Remove(r.Context(), core, r.PathValue("id")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listMineHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		list, err := svc.ListMine(r.Context(), core, callerID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]connectionResponse, 0, len(list))
		for _, c := range list {
			items = append(items, toConnectionResponse(c))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func setClassificationHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		var body struct {
			Classification string `json:"classification"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		c, err := svc.SetClassification(r.Context(), callerID, r.PathValue("id"), SetClassificationInput{Classification: Classification(body.Classification)})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"relationshipId": r.PathValue("id"), "classification": string(c)})
	}
}

type circleResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toCircleResponse(c Circle) circleResponse {
	return circleResponse{
		ID: c.ID, Name: c.Name, Status: c.Status,
		CreatedAt: c.CreatedAt.UTC().Format(timeFormat), UpdatedAt: c.UpdatedAt.UTC().Format(timeFormat),
	}
}

type circleMemberResponse struct {
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	IsManager bool   `json:"isManager"`
	CreatedAt string `json:"createdAt"`
}

func toCircleMemberResponse(m CircleMember) circleMemberResponse {
	return circleMemberResponse{UserID: m.UserID, Role: m.Role, IsManager: m.IsManager, CreatedAt: m.CreatedAt.UTC().Format(timeFormat)}
}

func callerAndGroups(w http.ResponseWriter, r *http.Request, newGroups GroupsFactory) (string, CoreGroups, bool) {
	callerID, ok := pulseauth.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
		return "", nil, false
	}
	client, ok := pulseauth.ClientFromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "core client missing from context"))
		return "", nil, false
	}
	return callerID, newGroups(client), true
}

func createCircleHandler(svc *Service, newGroups GroupsFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, groups, ok := callerAndGroups(w, r, newGroups)
		if !ok {
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		c, err := svc.CreateCircle(r.Context(), groups, CreateCircleInput{Name: body.Name})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toCircleResponse(c))
	}
}

func listCirclesHandler(svc *Service, newGroups GroupsFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, groups, ok := callerAndGroups(w, r, newGroups)
		if !ok {
			return
		}
		list, err := svc.ListMyCircles(r.Context(), groups)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]circleResponse, 0, len(list))
		for _, c := range list {
			items = append(items, toCircleResponse(c))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func listCircleMembersHandler(svc *Service, newGroups GroupsFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, groups, ok := callerAndGroups(w, r, newGroups)
		if !ok {
			return
		}
		list, err := svc.ListCircleMembers(r.Context(), groups, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]circleMemberResponse, 0, len(list))
		for _, m := range list {
			items = append(items, toCircleMemberResponse(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

// addCircleMemberHandler needs both CoreGroups (to actually add the
// member) and CoreRelationships (to check the caller and the target are
// really connected first - see Service.AddCircleMember's own doc
// comment).
func addCircleMemberHandler(svc *Service, newGroups GroupsFactory, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, groups, ok := callerAndGroups(w, r, newGroups)
		if !ok {
			return
		}
		_, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		var body struct {
			UserID string `json:"userId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		m, err := svc.AddCircleMember(r.Context(), groups, core, r.PathValue("id"), body.UserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toCircleMemberResponse(m))
	}
}

func removeCircleMemberHandler(svc *Service, newGroups GroupsFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, groups, ok := callerAndGroups(w, r, newGroups)
		if !ok {
			return
		}
		if err := svc.RemoveCircleMember(r.Context(), groups, r.PathValue("id"), r.PathValue("userId")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeDomainError translates both this module's own errors and any
// error forwarded transparently from Core's real relationships API
// (via coresdk.APIError) into Pulse's own response - Core's actual
// answer (already-exists, forbidden, not-found) is never hidden behind
// a generic failure.
func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	var apiErr *coresdk.APIError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "connection not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrNotConnected):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.As(err, &apiErr):
		apperr.Write(w, r, apperr.New(apperr.Code(apiErr.Code), apiErr.Message))
	default:
		slog.Default().Error("unhandled pulseconnections error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
