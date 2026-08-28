package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (h *Handler) internalAuthorized(r *http.Request) bool {
	expected := strings.TrimSpace(h.serviceToken)
	if expected == "" {
		expected = strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
	}
	if expected == "" {
		expected = strings.TrimSpace(os.Getenv("TELEGRAM_INTERNAL_TOKEN"))
	}
	if expected == "" {
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Service-Token"))
	if got == "" {
		got = strings.TrimSpace(r.Header.Get("X-Internal-Token"))
	}
	return got != "" && got == expected
}

// CreateGuestTicket creates a pre-login ticket from Telegram bot (no JWT user).
func (h *Handler) CreateGuestTicket(w http.ResponseWriter, r *http.Request) {
	if !h.internalAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Email          string `json:"email"`
		Subject        string `json:"subject"`
		Message        string `json:"message"`
		Category       string `json:"category"`
		TelegramChatID int64  `json:"telegram_chat_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Subject = strings.TrimSpace(req.Subject)
	req.Message = strings.TrimSpace(req.Message)
	req.Category = strings.TrimSpace(req.Category)
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		writeError(w, http.StatusBadRequest, "valid email required")
		return
	}
	if req.Subject == "" || req.Message == "" {
		writeError(w, http.StatusBadRequest, "subject and message are required")
		return
	}
	if req.TelegramChatID == 0 {
		writeError(w, http.StatusBadRequest, "telegram_chat_id required")
		return
	}
	if req.Category == "" {
		req.Category = "login"
	}
	if !validTicketCategory(req.Category) {
		writeError(w, http.StatusBadRequest, "invalid category")
		return
	}

	if existing, err := h.store.OpenGuestTicketByChat(r.Context(), req.TelegramChatID); err == nil && existing != nil {
		writeError(w, http.StatusConflict, "open ticket already exists")
		return
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("guest ticket lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create ticket")
		return
	}

	ticket, err := h.store.CreateGuestTicket(r.Context(), req.Email, req.Subject, req.Message, req.Category, req.TelegramChatID)
	if err != nil {
		log.Printf("create guest ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create ticket")
		return
	}
	go notifyNewTicketOps(ticket, req.Message)
	writeJSON(w, http.StatusCreated, map[string]any{"ticket": ticketJSON(*ticket)})
}

// AddGuestMessage appends a client message to an open guest Telegram ticket.
func (h *Handler) AddGuestMessage(w http.ResponseWriter, r *http.Request) {
	if !h.internalAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		TelegramChatID int64  `json:"telegram_chat_id"`
		Message        string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.TelegramChatID == 0 || req.Message == "" {
		writeError(w, http.StatusBadRequest, "telegram_chat_id and message required")
		return
	}

	ticket, err := h.store.OpenGuestTicketByChat(r.Context(), req.TelegramChatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no open ticket")
			return
		}
		log.Printf("guest message lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add message")
		return
	}

	if err := h.store.AddClientMessage(r.Context(), ticket.ID, "", ticket.ClientEmail, req.Message, nil); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "ticket is closed")
			return
		}
		log.Printf("guest add message: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add message")
		return
	}
	go notifyClientReplyOps(ticket, req.Message)
	writeJSON(w, http.StatusCreated, map[string]any{"ticket_id": ticket.ID, "status": "ok"})
}

// LookupGuestTicket returns the open guest ticket for a Telegram chat (if any).
func (h *Handler) LookupGuestTicket(w http.ResponseWriter, r *http.Request) {
	if !h.internalAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	chatID := strings.TrimSpace(r.URL.Query().Get("telegram_chat_id"))
	if chatID == "" {
		writeError(w, http.StatusBadRequest, "telegram_chat_id required")
		return
	}
	var id int64
	if _, err := fmt.Sscan(chatID, &id); err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid telegram_chat_id")
		return
	}
	ticket, err := h.store.OpenGuestTicketByChat(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"ticket": nil})
			return
		}
		log.Printf("lookup guest ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticketJSON(*ticket)})
}
