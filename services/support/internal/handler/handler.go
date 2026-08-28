package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/support/internal/authn"
	"github.com/borishru-boop/testVPStrade/services/support/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/support/internal/store"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	store              *store.Store
	secret             string
	serviceToken       string
	attachmentsDir     string
	maxAttachmentBytes int64
}

func New(st *store.Store, secret, serviceToken, attachmentsDir string, maxAttachmentBytes int64) *Handler {
	return &Handler{
		store:              st,
		secret:             secret,
		serviceToken:       serviceToken,
		attachmentsDir:     attachmentsDir,
		maxAttachmentBytes: maxAttachmentBytes,
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "support"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Subject    string `json:"subject"`
		Message    string `json:"message"`
		Priority   string `json:"priority"`
		Category   string `json:"category"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Message = store.NormalizeMessageBody(req.Message)
	req.Category = strings.TrimSpace(req.Category)
	req.InstanceID = strings.TrimSpace(req.InstanceID)
	if req.Subject == "" || store.IsEmptyMessageBody(req.Message) {
		writeError(w, http.StatusBadRequest, "subject and message are required")
		return
	}
	if req.InstanceID == "" {
		hasInstance, err := h.store.UserHasActiveInstance(r.Context(), claims.UserID)
		if err != nil {
			log.Printf("create ticket instance check: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to create ticket")
			return
		}
		if hasInstance {
			writeError(w, http.StatusBadRequest, "instance_id is required")
			return
		}
	} else {
		owns, err := h.store.UserOwnsInstance(r.Context(), claims.UserID, req.InstanceID)
		if err != nil {
			log.Printf("create ticket ownership check: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to create ticket")
			return
		}
		if !owns {
			writeError(w, http.StatusBadRequest, "instance not found")
			return
		}
	}
	if req.Category == "" {
		req.Category = "other"
	}
	if !validTicketCategory(req.Category) {
		writeError(w, http.StatusBadRequest, "invalid category")
		return
	}

	ticket, err := h.store.CreateTicket(r.Context(), claims.UserID, claims.Email, req.Subject, req.Message, req.Priority, req.Category, req.InstanceID)
	if err != nil {
		log.Printf("create ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create ticket")
		return
	}
	go notifyNewTicketOps(ticket, req.Message)
	writeJSON(w, http.StatusCreated, map[string]any{"ticket": ticketJSON(*ticket)})
}

func (h *Handler) ListTickets(w http.ResponseWriter, r *http.Request) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	items, err := h.store.ListUserTickets(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("list tickets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list tickets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": ticketsJSON(items)})
}

func (h *Handler) GetTicket(w http.ResponseWriter, r *http.Request) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ticketID := r.PathValue("id")
	ticket, err := h.store.GetTicket(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		log.Printf("get ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load ticket")
		return
	}
	if !authn.IsStaff(claims.Roles) && (ticket.UserID == "" || ticket.UserID != claims.UserID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	messages, err := h.store.ListMessages(r.Context(), ticketID)
	if err != nil {
		log.Printf("list messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}
	attachments, err := h.store.ListAttachmentsForTicket(r.Context(), ticketID)
	if err != nil {
		log.Printf("list attachments: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load attachments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":      ticketJSON(*ticket),
		"messages":    messagesJSON(messages, !authn.IsStaff(claims.Roles)),
		"attachments": attachmentsJSON(attachments),
	})
}

func (h *Handler) AddMessage(w http.ResponseWriter, r *http.Request) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ticketID := r.PathValue("id")
	ticket, err := h.store.GetTicket(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		log.Printf("get ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load ticket")
		return
	}
	if ticket.UserID == "" || ticket.UserID != claims.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if ticket.Status == "closed" {
		writeError(w, http.StatusConflict, "ticket is closed")
		return
	}

	var req struct {
		Body          string   `json:"body"`
		AttachmentIDs []string `json:"attachment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Body = store.NormalizeMessageBody(req.Body)
	if store.IsEmptyMessageBody(req.Body) && len(req.AttachmentIDs) == 0 {
		writeError(w, http.StatusBadRequest, "body or attachment_ids required")
		return
	}

	if err := h.store.AddClientMessage(r.Context(), ticketID, claims.UserID, claims.Email, req.Body, req.AttachmentIDs); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "ticket is closed")
			return
		}
		if strings.Contains(err.Error(), "attachment") {
			writeError(w, http.StatusBadRequest, "attachment not found or already linked")
			return
		}
		log.Printf("add message: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add message")
		return
	}
	go notifyClientReplyOps(ticket, req.Body)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *Handler) CloseTicket(w http.ResponseWriter, r *http.Request) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ticketID := r.PathValue("id")
	ticket, err := h.store.GetTicket(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		log.Printf("get ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load ticket")
		return
	}
	if ticket.UserID == "" || ticket.UserID != claims.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if ticket.Status != "answered" {
		writeError(w, http.StatusConflict, "ticket cannot be closed")
		return
	}

	closed, err := h.store.ClientCloseTicket(r.Context(), ticketID, claims.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "ticket cannot be closed")
			return
		}
		log.Printf("close ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to close ticket")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticketJSON(*closed)})
}

func (h *Handler) AdminListTickets(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID != "" {
		items, err := h.store.ListUserTickets(r.Context(), userID)
		if err != nil {
			log.Printf("admin list user tickets: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to list tickets")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tickets": ticketsJSON(items)})
		return
	}

	filter := r.URL.Query().Get("filter")
	if filter != "" && filter != "all" && filter != "queue" && filter != "mine" {
		writeError(w, http.StatusBadRequest, "invalid filter")
		return
	}
	if filter == "" || filter == "all" {
		filter = ""
	}
	statusFilter := r.URL.Query().Get("status")
	if statusFilter != "" && !validTicketStatus(statusFilter) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	items, err := h.store.ListStaffTickets(r.Context(), filter, statusFilter, claims.UserID)
	if err != nil {
		if errors.Is(err, store.ErrInvalidTicketStatus) {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		log.Printf("admin list tickets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list tickets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": ticketsJSON(items)})
}

func (h *Handler) AdminClaimNext(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	ticket, err := h.store.ClaimNext(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("claim next: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to claim ticket")
		return
	}
	if ticket == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ticket": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticketJSON(*ticket)})
}

func (h *Handler) AdminTake(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	ticketID := r.PathValue("id")
	ticket, err := h.store.TakeTicket(r.Context(), ticketID, claims.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "ticket unavailable")
			return
		}
		log.Printf("take ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to take ticket")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticketJSON(*ticket)})
}

func (h *Handler) AdminRelease(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	ticketID := r.PathValue("id")
	ticket, err := h.store.ReleaseTicket(r.Context(), ticketID, claims.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "ticket unavailable")
			return
		}
		log.Printf("release ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to release ticket")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticketJSON(*ticket)})
}

func (h *Handler) AdminUpdateMessage(w http.ResponseWriter, r *http.Request) {
	if _, err := h.staffClaims(w, r); err != nil {
		return
	}
	ticketID := r.PathValue("id")
	messageID := r.PathValue("messageId")
	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	msg, err := h.store.UpdateMessage(r.Context(), ticketID, messageID, req.Body)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		if strings.Contains(err.Error(), "empty body") {
			writeError(w, http.StatusBadRequest, "body required")
			return
		}
		log.Printf("update message: %v", err)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": messageJSON(*msg, false)})
}

func (h *Handler) AdminDeleteMessage(w http.ResponseWriter, r *http.Request) {
	if _, err := h.staffClaims(w, r); err != nil {
		return
	}
	ticketID := r.PathValue("id")
	messageID := r.PathValue("messageId")
	if err := h.store.DeleteMessage(r.Context(), ticketID, messageID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		log.Printf("delete message: %v", err)
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) AdminReplyClose(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	ticketID := r.PathValue("id")
	var req struct {
		Body          string   `json:"body"`
		Mode          string   `json:"mode"`
		AttachmentIDs []string `json:"attachment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Body = store.NormalizeMessageBody(req.Body)
	req.Mode = strings.TrimSpace(req.Mode)
	if store.IsEmptyMessageBody(req.Body) && len(req.AttachmentIDs) == 0 {
		writeError(w, http.StatusBadRequest, "body or attachment_ids required")
		return
	}
	if req.Mode != "answered" && req.Mode != "immediate" {
		writeError(w, http.StatusBadRequest, "mode must be answered or immediate")
		return
	}

	ticket, err := h.store.StaffReplyAndClose(r.Context(), ticketID, claims.UserID, claims.Email, req.Body, req.Mode, req.AttachmentIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "ticket unavailable")
			return
		}
		if strings.Contains(err.Error(), "attachment") {
			writeError(w, http.StatusBadRequest, "attachment not found or already linked")
			return
		}
		log.Printf("reply close: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to close ticket")
		return
	}
	if req.Mode == "answered" || req.Mode == "immediate" {
		go notifyStaffReply(ticket, req.Body, req.Mode)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticketJSON(*ticket)})
}

func (h *Handler) AdminShiftStatus(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	shift, err := h.store.GetShift(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("shift status: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load shift")
		return
	}
	cfg, _ := h.store.GetSlotConfig(r.Context())
	usage, _ := h.store.CountSlots(r.Context(), claims.UserID)
	writeJSON(w, http.StatusOK, map[string]any{
		"shift": shiftJSON(shift),
		"slots": map[string]any{
			"max_new":             cfg.MaxNewSlots,
			"max_return":          cfg.MaxReturnSlots,
			"new_in_progress":     usage.NewInProgress,
			"return_in_progress":  usage.ReturnInProgress,
			"return_pending":      usage.ReturnPending,
		},
	})
}

func (h *Handler) AdminShiftStart(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	shift, err := h.store.SetShiftOnDuty(r.Context(), claims.UserID, true)
	if err != nil {
		log.Printf("shift start: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to start shift")
		return
	}
	claimed, _ := h.store.FillSlots(r.Context(), claims.UserID)
	writeJSON(w, http.StatusOK, map[string]any{
		"shift":    shiftJSON(shift),
		"assigned": ticketsJSON(claimedToSlice(claimed)),
	})
}

func (h *Handler) AdminShiftEnd(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	shift, err := h.store.SetShiftOnDuty(r.Context(), claims.UserID, false)
	if err != nil {
		log.Printf("shift end: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to end shift")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shift": shiftJSON(shift)})
}

func (h *Handler) AdminWorkspace(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	shift, _ := h.store.GetShift(r.Context(), claims.UserID)
	if shift.OnDuty {
		_, _ = h.store.FillSlots(r.Context(), claims.UserID)
	}
	cfg, _ := h.store.GetSlotConfig(r.Context())
	usage, _ := h.store.CountSlots(r.Context(), claims.UserID)
	tickets, err := h.store.GetWorkspace(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("workspace: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load workspace")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"shift":    shiftJSON(shift),
		"slots": map[string]any{
			"max_new":            cfg.MaxNewSlots,
			"max_return":         cfg.MaxReturnSlots,
			"new_in_progress":    usage.NewInProgress,
			"return_in_progress": usage.ReturnInProgress,
			"return_pending":     usage.ReturnPending,
		},
		"tickets": ticketsJSON(tickets),
	})
}

func (h *Handler) AdminSlotConfig(w http.ResponseWriter, r *http.Request) {
	claims, err := h.staffClaims(w, r)
	if err != nil {
		return
	}
	if r.Method == http.MethodGet {
		cfg, err := h.store.GetSlotConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load slot config")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"config": slotConfigJSON(cfg)})
		return
	}
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !authn.IsManager(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req struct {
		MaxNewSlots    *int `json:"max_new_slots"`
		MaxReturnSlots *int `json:"max_return_slots"`
		RebindHours    *int `json:"rebind_hours"`
		AutoCloseHours *int `json:"auto_close_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	current, err := h.store.GetSlotConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load slot config")
		return
	}
	if req.MaxNewSlots != nil {
		current.MaxNewSlots = *req.MaxNewSlots
	}
	if req.MaxReturnSlots != nil {
		current.MaxReturnSlots = *req.MaxReturnSlots
	}
	if req.RebindHours != nil {
		current.RebindHours = *req.RebindHours
	}
	if req.AutoCloseHours != nil {
		current.AutoCloseHours = *req.AutoCloseHours
	}
	updated, err := h.store.UpdateSlotConfig(r.Context(), current)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": slotConfigJSON(updated)})
}

func claimedToSlice(items []*store.Ticket) []store.Ticket {
	out := make([]store.Ticket, 0, len(items))
	for _, t := range items {
		if t != nil {
			out = append(out, *t)
		}
	}
	return out
}

func (h *Handler) AdminUpdate(w http.ResponseWriter, r *http.Request) {
	_, err := h.staffClaims(w, r)
	if err != nil {
		return
	}

	ticketID := r.PathValue("id")
	var req struct {
		Status   string `json:"status"`
		Priority string `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Status != "" && req.Status != "closed" && req.Status != "resolved" {
		writeError(w, http.StatusBadRequest, "only closed or resolved status allowed via patch")
		return
	}

	before, _ := h.store.GetTicket(r.Context(), ticketID)
	ticket, err := h.store.UpdateTicket(r.Context(), ticketID, req.Status, req.Priority)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		log.Printf("update ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update ticket")
		return
	}
	if (req.Status == "closed" || req.Status == "resolved") && before != nil && before.AssigneeID != nil {
		_, _ = h.store.FillSlots(r.Context(), *before.AssigneeID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticketJSON(*ticket)})
}

func (h *Handler) claims(r *http.Request) (*authn.Claims, error) {
	return authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
}

func (h *Handler) staffClaims(w http.ResponseWriter, r *http.Request) (*authn.Claims, error) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, err
	}
	if !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil, errors.New("forbidden")
	}
	return claims, nil
}

func ticketJSON(t store.Ticket) map[string]any {
	out := map[string]any{
		"id":                    t.ID,
		"user_id":               t.UserID,
		"client_email":          t.ClientEmail,
		"subject":               t.Subject,
		"status":                t.Status,
		"priority":              t.Priority,
		"category":              t.Category,
		"sla_due_at":            t.SLADueAt.UTC().Format(time.RFC3339),
		"created_at":            t.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":            t.UpdatedAt.UTC().Format(time.RFC3339),
		"last_message_at":       t.LastMessageAt.UTC().Format(time.RFC3339),
		"last_message_by_staff": t.LastMessageByStaff,
		"is_return":             t.IsReturn,
	}
	if t.AssigneeID != nil && *t.AssigneeID != "" {
		out["assignee_id"] = *t.AssigneeID
	}
	if t.InstanceID != nil && *t.InstanceID != "" {
		out["instance_id"] = *t.InstanceID
	}
	if t.AnsweredAt != nil {
		out["answered_at"] = t.AnsweredAt.UTC().Format(time.RFC3339)
	}
	if t.AutoCloseAt != nil {
		out["auto_close_at"] = t.AutoCloseAt.UTC().Format(time.RFC3339)
	}
	if t.TelegramChatID != nil && *t.TelegramChatID != 0 {
		out["telegram_chat_id"] = *t.TelegramChatID
	}
	return out
}

func shiftJSON(sh store.StaffShift) map[string]any {
	out := map[string]any{
		"staff_id": sh.StaffID,
		"on_duty":  sh.OnDuty,
	}
	if sh.StartedAt != nil {
		out["started_at"] = sh.StartedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func slotConfigJSON(cfg store.SlotConfig) map[string]any {
	return map[string]any{
		"max_new_slots":    cfg.MaxNewSlots,
		"max_return_slots": cfg.MaxReturnSlots,
		"rebind_hours":     cfg.RebindHours,
		"auto_close_hours": cfg.AutoCloseHours,
	}
}

func validTicketCategory(category string) bool {
	switch category {
	case "login", "network", "password", "performance", "billing", "other":
		return true
	default:
		return false
	}
}

func ticketAdminURL(ticketID string) string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("SITE_URL")), "/")
	if base == "" {
		base = "https://cloud-hustle.com"
	}
	return base + "/vps/admin/tickets/" + ticketID
}

func opsTicketAlertBody(ticket *store.Ticket, message string) string {
	preview := strings.TrimSpace(message)
	if len([]rune(preview)) > 280 {
		preview = string([]rune(preview)[:280]) + "…"
	}
	instanceID := "—"
	if ticket.InstanceID != nil && *ticket.InstanceID != "" {
		instanceID = *ticket.InstanceID
	}
	userLabel := ticket.UserID
	if userLabel == "" {
		userLabel = "guest"
	}
	tg := "—"
	if ticket.TelegramChatID != nil && *ticket.TelegramChatID != 0 {
		tg = fmt.Sprintf("%d", *ticket.TelegramChatID)
	}
	return fmt.Sprintf(
		"Тикет: %s\nТема: %s\nEmail: %s\nUser: %s\nTelegram: %s\nКатегория: %s\nПриоритет: %s\nСервер: %s\n\n%s\n\nОткрыть: %s",
		ticket.ID, ticket.Subject, ticket.ClientEmail, userLabel, tg,
		ticket.Category, ticket.Priority, instanceID, preview, ticketAdminURL(ticket.ID),
	)
}

func notifyNewTicketOps(ticket *store.Ticket, message string) {
	if ticket == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	if err := notify.OpsAlert(ctx, "🎫 Новый тикет", opsTicketAlertBody(ticket, message)); err != nil {
		log.Printf("new ticket ops alert: %v", err)
	}
}

func notifyClientReplyOps(ticket *store.Ticket, message string) {
	if ticket == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	if err := notify.OpsAlert(ctx, "💬 Ответ клиента", opsTicketAlertBody(ticket, message)); err != nil {
		log.Printf("client reply ops alert: %v", err)
	}
}

func notifyStaffReply(ticket *store.Ticket, replyBody, mode string) {
	if ticket == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	hours := 12
	if ticket.AnsweredAt != nil && ticket.AutoCloseAt != nil {
		if h := int(ticket.AutoCloseAt.Sub(*ticket.AnsweredAt).Hours()); h > 0 {
			hours = h
		}
	}
	title := "Ответ поддержки"
	body := strings.TrimSpace(replyBody)
	if body == "" {
		body = "По вашему обращению есть ответ."
	}
	tgBody := body
	if mode == "answered" {
		tgBody = fmt.Sprintf("%s\n\nТикет автоматически переместится в архив через %d ч. Можете ответить здесь в Telegram или в личном кабинете.", body, hours)
	}

	if ticket.TelegramChatID != nil && *ticket.TelegramChatID != 0 {
		if err := notify.TelegramChat(ctx, *ticket.TelegramChatID, title, tgBody); err != nil {
			log.Printf("notify ticket telegram chat=%d: %v", *ticket.TelegramChatID, err)
		}
	}
	if ticket.UserID != "" {
		if mode == "answered" {
			if err := notify.TicketAnswered(ctx, ticket.UserID, hours, replyBody); err != nil {
				log.Printf("notify ticket answered user=%s: %v", ticket.UserID, err)
			}
		} else if err := notify.User(ctx, ticket.UserID, title, body, "support", true); err != nil {
			log.Printf("notify ticket closed user=%s: %v", ticket.UserID, err)
		}
	}
}

func validTicketStatus(status string) bool {
	switch status {
	case "open", "in_progress", "waiting_client", "answered", "return_pending", "resolved", "closed":
		return true
	default:
		return false
	}
}

func ticketsJSON(items []store.Ticket) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, t := range items {
		out = append(out, ticketJSON(t))
	}
	return out
}

func messagesJSON(items []store.Message, redactStaffEmails bool) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, m := range items {
		out = append(out, messageJSON(m, redactStaffEmails))
	}
	return out
}

func messageJSON(m store.Message, redactStaffEmails bool) map[string]any {
	authorEmail := m.AuthorEmail
	if redactStaffEmails && m.IsStaff {
		authorEmail = ""
	}
	row := map[string]any{
		"id":           m.ID,
		"ticket_id":    m.TicketID,
		"author_id":    m.AuthorID,
		"author_email": authorEmail,
		"is_staff":     m.IsStaff,
		"body":         m.Body,
		"created_at":   m.CreatedAt.UTC().Format(time.RFC3339),
	}
	if m.EditedAt != nil {
		row["edited_at"] = m.EditedAt.UTC().Format(time.RFC3339)
	}
	return row
}

func attachmentsJSON(items []store.Attachment) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		row := map[string]any{
			"id":           a.ID,
			"ticket_id":    a.TicketID,
			"filename":     a.Filename,
			"content_type": a.ContentType,
			"size_bytes":   a.SizeBytes,
			"created_at":   a.CreatedAt.UTC().Format(time.RFC3339),
		}
		if a.MessageID != nil && *a.MessageID != "" {
			row["message_id"] = *a.MessageID
		}
		out = append(out, row)
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
