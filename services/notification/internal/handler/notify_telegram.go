package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// NotifyTelegram sends a Bot API message to an arbitrary chat_id (guest support tickets).
func (h *Handler) NotifyTelegram(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.Header.Get("X-Service-Token"))
	expected := os.Getenv("NOTIFICATION_SERVICE_TOKEN")
	if expected == "" || token != expected {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		TelegramChatID int64  `json:"telegram_chat_id"`
		Title          string `json:"title"`
		Body           string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.TelegramChatID == 0 {
		writeError(w, http.StatusBadRequest, "telegram_chat_id required")
		return
	}
	if req.Body == "" && req.Title == "" {
		writeError(w, http.StatusBadRequest, "title or body required")
		return
	}
	if req.Title == "" {
		req.Title = "Поддержка"
	}

	if h.telegram == nil {
		log.Printf("notify-telegram skipped (bot token not configured) chat=%d", req.TelegramChatID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": false, "reason": "telegram_not_configured"})
		return
	}
	if err := h.telegram.SendText(r.Context(), req.TelegramChatID, req.Title, req.Body); err != nil {
		log.Printf("notify-telegram chat=%d: %v", req.TelegramChatID, err)
		writeError(w, http.StatusBadGateway, "telegram send failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": true})
}
