package messaging

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

// RegisterRoutes wires the conversation/message endpoints, all requiring
// an authenticated caller. Per-conversation membership authorization
// happens inside Service, not here - the same split every other module in
// this repo uses.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/conversations", requireUser(createConversationHandler(svc)))
	mux.Handle("GET /v1/conversations", requireUser(listMineHandler(svc)))
	mux.Handle("GET /v1/conversations/{id}", requireUser(getConversationHandler(svc)))
	mux.Handle("GET /v1/conversations/{id}/members", requireUser(listMembersHandler(svc)))
	mux.Handle("POST /v1/conversations/{id}/members", requireUser(addMemberHandler(svc)))
	mux.Handle("DELETE /v1/conversations/{id}/members/{userId}", requireUser(removeMemberHandler(svc)))

	mux.Handle("POST /v1/conversations/{id}/messages", requireUser(sendMessageHandler(svc)))
	mux.Handle("GET /v1/conversations/{id}/messages", requireUser(listMessagesHandler(svc)))
	mux.Handle("GET /v1/conversations/{id}/messages/{messageId}", requireUser(getMessageHandler(svc)))
	mux.Handle("POST /v1/conversations/{id}/messages/{messageId}/delivered", requireUser(markDeliveredHandler(svc)))

	mux.Handle("PUT /v1/conversations/{id}/read", requireUser(setReadStateHandler(svc)))
	mux.Handle("GET /v1/conversations/{id}/read/{userId}", requireUser(getReadStateHandler(svc)))

	mux.Handle("POST /v1/conversations/{id}/messages/{messageId}/reactions", requireUser(addReactionHandler(svc)))
	mux.Handle("GET /v1/conversations/{id}/messages/{messageId}/reactions", requireUser(listReactionsHandler(svc)))
	mux.Handle("DELETE /v1/conversations/{id}/messages/{messageId}/reactions/{type}", requireUser(removeReactionHandler(svc)))
}

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

type conversationResponse struct {
	ID        string           `json:"id"`
	AppID     string           `json:"appId"`
	Type      ConversationType `json:"type"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt"`
}

func toConversationResponse(c Conversation) conversationResponse {
	return conversationResponse{
		ID: c.ID, AppID: c.AppID, Type: c.Type, Metadata: c.Metadata,
		CreatedAt: c.CreatedAt.UTC().Format(timeFormat),
		UpdatedAt: c.UpdatedAt.UTC().Format(timeFormat),
	}
}

type memberResponse struct {
	ConversationID string `json:"conversationId"`
	UserID         string `json:"userId"`
	JoinedAt       string `json:"joinedAt"`
}

func toMemberResponse(m ConversationMember) memberResponse {
	return memberResponse{
		ConversationID: m.ConversationID, UserID: m.UserID,
		JoinedAt: m.JoinedAt.UTC().Format(timeFormat),
	}
}

type attachmentResponse struct {
	ID          string         `json:"id"`
	URL         string         `json:"url"`
	ContentType string         `json:"contentType"`
	SizeBytes   int64          `json:"sizeBytes"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type messageResponse struct {
	ID             string               `json:"id"`
	ConversationID string               `json:"conversationId"`
	SenderID       string               `json:"senderId"`
	Type           string               `json:"type"`
	Body           map[string]any       `json:"body,omitempty"`
	Attachments    []attachmentResponse `json:"attachments,omitempty"`
	CreatedAt      string               `json:"createdAt"`
}

func toMessageResponse(m Message) messageResponse {
	resp := messageResponse{
		ID: m.ID, ConversationID: m.ConversationID, SenderID: m.SenderID, Type: m.Type, Body: m.Body,
		CreatedAt: m.CreatedAt.UTC().Format(timeFormat),
	}
	for _, a := range m.Attachments {
		resp.Attachments = append(resp.Attachments, attachmentResponse{
			ID: a.ID, URL: a.URL, ContentType: a.ContentType, SizeBytes: a.SizeBytes, Metadata: a.Metadata,
		})
	}
	return resp
}

type deliveryResponse struct {
	MessageID   string  `json:"messageId"`
	UserID      string  `json:"userId"`
	DeliveredAt *string `json:"deliveredAt,omitempty"`
}

func toDeliveryResponse(d Delivery) deliveryResponse {
	resp := deliveryResponse{MessageID: d.MessageID, UserID: d.UserID}
	if d.DeliveredAt != nil {
		s := d.DeliveredAt.UTC().Format(timeFormat)
		resp.DeliveredAt = &s
	}
	return resp
}

type readStateResponse struct {
	ConversationID  string  `json:"conversationId"`
	UserID          string  `json:"userId"`
	LastReadMessage string  `json:"lastReadMessageId,omitempty"`
	LastReadAt      *string `json:"lastReadAt,omitempty"`
}

func toReadStateResponse(rs ReadState) readStateResponse {
	resp := readStateResponse{ConversationID: rs.ConversationID, UserID: rs.UserID, LastReadMessage: rs.LastReadMessage}
	if rs.LastReadAt != nil {
		s := rs.LastReadAt.UTC().Format(timeFormat)
		resp.LastReadAt = &s
	}
	return resp
}

type reactionResponse struct {
	ID        string `json:"id"`
	MessageID string `json:"messageId"`
	UserID    string `json:"userId"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
}

func toReactionResponse(r Reaction) reactionResponse {
	return reactionResponse{ID: r.ID, MessageID: r.MessageID, UserID: r.UserID, Type: r.Type, CreatedAt: r.CreatedAt.UTC().Format(timeFormat)}
}

func callerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	u, ok := users.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
		return "", false
	}
	return u.ID, true
}

func createConversationHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID         string           `json:"appId"`
			Type          ConversationType `json:"type"`
			MemberUserIDs []string         `json:"memberUserIds"`
			Metadata      map[string]any   `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		// The creator is always a member even if the client forgot to list
		// themselves - mirroring groups.Create's creator-is-always-manager
		// guarantee.
		members := body.MemberUserIDs
		if !containsID(members, caller) {
			members = append(members, caller)
		}
		c, err := svc.CreateConversation(r.Context(), caller,
			CreateConversationInput{AppID: body.AppID, Type: body.Type, MemberUserIDs: members, Metadata: body.Metadata})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toConversationResponse(c))
	}
}

func containsID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListMine(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]conversationResponse, 0, len(list))
		for _, c := range list {
			items = append(items, toConversationResponse(c))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getConversationHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		c, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toConversationResponse(c))
	}
}

func listMembersHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListMembers(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]memberResponse, 0, len(list))
		for _, m := range list {
			items = append(items, toMemberResponse(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func addMemberHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
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
		if body.UserID == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "userId is required"))
			return
		}
		m, err := svc.AddMember(r.Context(), caller, r.PathValue("id"), body.UserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toMemberResponse(m))
	}
}

func removeMemberHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if err := svc.RemoveMember(r.Context(), caller, r.PathValue("id"), r.PathValue("userId")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func sendMessageHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Type        string           `json:"type"`
			Body        map[string]any   `json:"body"`
			Attachments []attachmentBody `json:"attachments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		attachments := make([]AttachmentInput, 0, len(body.Attachments))
		for _, a := range body.Attachments {
			attachments = append(attachments, AttachmentInput{URL: a.URL, ContentType: a.ContentType, SizeBytes: a.SizeBytes, Metadata: a.Metadata})
		}
		msg, err := svc.SendMessage(r.Context(), caller, r.PathValue("id"),
			SendMessageInput{Type: body.Type, Body: body.Body, Attachments: attachments})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toMessageResponse(msg))
	}
}

type attachmentBody struct {
	URL         string         `json:"url"`
	ContentType string         `json:"contentType"`
	SizeBytes   int64          `json:"sizeBytes"`
	Metadata    map[string]any `json:"metadata"`
}

func listMessagesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		result, err := svc.ListMessages(r.Context(), caller, r.PathValue("id"),
			ListParams{Limit: limit, Cursor: r.URL.Query().Get("cursor")})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]messageResponse, 0, len(result.Items))
		for _, m := range result.Items {
			items = append(items, toMessageResponse(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
	}
}

func getMessageHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		msg, err := svc.GetMessage(r.Context(), caller, r.PathValue("messageId"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toMessageResponse(msg))
	}
}

func markDeliveredHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		d, err := svc.MarkDelivered(r.Context(), caller, r.PathValue("messageId"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toDeliveryResponse(d))
	}
}

func setReadStateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			LastReadMessageID string `json:"lastReadMessageId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		if body.LastReadMessageID == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "lastReadMessageId is required"))
			return
		}
		rs, err := svc.SetReadState(r.Context(), caller, r.PathValue("id"), body.LastReadMessageID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toReadStateResponse(rs))
	}
}

func getReadStateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		rs, err := svc.GetReadState(r.Context(), caller, r.PathValue("id"), r.PathValue("userId"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toReadStateResponse(rs))
	}
}

func addReactionHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		if body.Type == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "type is required"))
			return
		}
		reaction, err := svc.AddReaction(r.Context(), caller, r.PathValue("messageId"), body.Type)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toReactionResponse(reaction))
	}
}

func listReactionsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListReactions(r.Context(), caller, r.PathValue("messageId"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]reactionResponse, 0, len(list))
		for _, rc := range list {
			items = append(items, toReactionResponse(rc))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func removeReactionHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if err := svc.RemoveReaction(r.Context(), caller, r.PathValue("messageId"), r.PathValue("type")); err != nil {
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
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrMessageNotFound), errors.Is(err, ErrMembershipNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrAlreadyMember):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrMembershipFixed), errors.Is(err, ErrCannotRemoveSelf):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled messaging error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
