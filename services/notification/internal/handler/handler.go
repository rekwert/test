package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/notification/internal/authn"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/mailer"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/store"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/telegram"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/templates"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	mailer       *mailer.Mailer
	store        *store.Store
	secret       string
	redis        RedisPublisher
	telegram     *telegram.Client // user pushes (linked accounts)
	alerts       *telegram.Client // ops/alerts channel (monitoring chat)
	alertsChatID int64
}

// RedisPublisher publishes wake events for the email worker.
type RedisPublisher interface {
	Publish(ctx context.Context, channel, message string) error
}

func New(m *mailer.Mailer, st *store.Store, jwtSecret string, tg, alerts *telegram.Client, alertsChatID int64) *Handler {
	return &Handler{
		mailer:       m,
		store:        st,
		secret:       jwtSecret,
		telegram:     tg,
		alerts:       alerts,
		alertsChatID: alertsChatID,
	}
}

func (h *Handler) SetRedisPublisher(r RedisPublisher) {
	h.redis = r
}

const emailWakeChannel = "notification:email"

func (h *Handler) enqueueEmail(ctx context.Context, userID *string, to, subject, body, template string, metadata json.RawMessage) error {
	if h.store == nil {
		return pgx.ErrNoRows
	}
	_, err := h.store.EnqueueEmail(ctx, userID, to, subject, body, template, metadata)
	if err != nil {
		return err
	}
	if h.redis != nil {
		_ = h.redis.Publish(ctx, emailWakeChannel, "1")
	}
	return nil
}

type sendRequest struct {
	To       string            `json:"to"`
	Template string            `json:"template"`
	Locale   string            `json:"locale"`
	Data     map[string]string `json:"data"`
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	if !serviceTokenOK(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.To = strings.TrimSpace(strings.ToLower(req.To))
	req.Locale = strings.ToLower(req.Locale)
	if req.Locale != "ru" {
		req.Locale = "en"
	}
	code := req.Data["code"]
	if req.To == "" || req.Template == "" || len(code) != 6 {
		writeError(w, http.StatusBadRequest, "to, template and 6-digit code required")
		return
	}

	subject, body, htmlBody, err := templates.Render(req.Template, req.Locale, code)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	meta := map[string]string{
		"code":   code,
		"locale": req.Locale,
	}
	if !templates.IsTransactional(req.Template) {
		meta["html_body"] = htmlBody
	}
	metaJSON, _ := json.Marshal(meta)
	if err := h.enqueueEmail(r.Context(), nil, req.To, subject, body, req.Template, metaJSON); err != nil {
		if h.store == nil {
			if err := h.mailer.Send(mailer.Message{
				To:            req.To,
				Subject:       subject,
				Body:          body,
				HTML:          htmlBody,
				Transactional: templates.IsTransactional(req.Template),
			}); err != nil {
				writeError(w, http.StatusInternalServerError, "send failed")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
			return
		}
		writeError(w, http.StatusInternalServerError, "queue failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"notifications": []any{}})
		return
	}
	items, err := h.store.ListInbox(r.Context(), userID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := map[string]any{
			"id":         item.ID,
			"title":      item.Title,
			"body":       item.Body,
			"category":   item.Category,
			"read":       item.ReadAt != nil,
			"created_at": item.CreatedAt,
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
}

func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := h.store.MarkRead(r.Context(), userID, id); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": 0})
		return
	}
	n, err := h.store.MarkAllRead(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": n})
}

func (h *Handler) AdminSendNotification(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	staff := authn.IsStaff(claims.Roles)
	if !staff && h.store != nil {
		staff, err = h.store.UserIsStaff(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "auth check failed")
			return
		}
	}
	if !staff {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}

	var req struct {
		NodeID    string `json:"node_id"`
		UserID    string `json:"user_id"`
		UserEmail string `json:"user_email"`
		Message   string `json:"message"`
		Title     string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.UserEmail = strings.TrimSpace(strings.ToLower(req.UserEmail))
	req.Message = strings.TrimSpace(req.Message)
	req.Title = strings.TrimSpace(req.Title)
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message required")
		return
	}
	if req.Title == "" {
		req.Title = "Уведомление"
	}

	hasNode := req.NodeID != ""
	hasUser := req.UserID != "" || req.UserEmail != ""
	if hasNode == hasUser {
		writeError(w, http.StatusBadRequest, "provide either node_id or user_id/user_email")
		return
	}

	var userIDs []string
	if hasUser {
		targetID := req.UserID
		if req.UserEmail != "" {
			id, err := h.store.UserIDByEmail(r.Context(), req.UserEmail)
			if err != nil {
				if err == pgx.ErrNoRows {
					writeError(w, http.StatusNotFound, "user not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "lookup failed")
				return
			}
			targetID = id
		} else if _, err := uuid.Parse(targetID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		exists, err := h.store.UserExists(r.Context(), targetID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		userIDs = []string{targetID}
	} else {
		if _, err := uuid.Parse(req.NodeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid node_id")
			return
		}
		exists, err := h.store.NodeExists(r.Context(), req.NodeID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		userIDs, err = h.store.ListUserIDsByNode(r.Context(), req.NodeID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if len(userIDs) == 0 {
			writeError(w, http.StatusNotFound, "no active users on this node")
			return
		}
	}

	sent, err := h.store.CreateInboxMessages(r.Context(), userIDs, req.Title, req.Message, "admin", claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "send failed")
		return
	}

	for _, uid := range userIDs {
		email, err := h.store.UserEmailByID(r.Context(), uid)
		if err != nil || email == "" {
			continue
		}
		_ = h.enqueueEmail(r.Context(), &uid, email, req.Title, req.Message, "", nil)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"sent":  sent,
		"users": userIDs,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serviceTokenOK(r *http.Request) bool {
	got := strings.TrimSpace(r.Header.Get("X-Service-Token"))
	if got == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			got = strings.TrimSpace(auth[7:])
		}
	}
	want := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
	if want == "" {
		want = strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
	}
	return want != "" && got == want
}
