package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

func (h *Handler) SystemNotify(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.Header.Get("X-Service-Token"))
	expected := os.Getenv("NOTIFICATION_SERVICE_TOKEN")
	if expected == "" || token != expected {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}

	var req struct {
		UserID        string `json:"user_id"`
		Title         string `json:"title"`
		Body          string `json:"body"`
		HTMLBody      string `json:"html_body"`
		Category      string `json:"category"`
		SendEmail     bool   `json:"send_email"`
		SendTelegram  *bool  `json:"send_telegram"` // nil = auto (respect user prefs)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.HTMLBody = strings.TrimSpace(req.HTMLBody)
	if req.UserID == "" || req.Body == "" {
		writeError(w, http.StatusBadRequest, "user_id and body required")
		return
	}
	if req.Title == "" {
		req.Title = "Уведомление"
	}
	if req.Category == "" {
		req.Category = "system"
	}

	if _, err := h.store.CreateInboxMessages(r.Context(), []string{req.UserID}, req.Title, req.Body, req.Category, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "inbox failed")
		return
	}

	if req.SendEmail {
		email, err := h.store.UserEmailByID(r.Context(), req.UserID)
		if err == nil && email != "" {
			var meta json.RawMessage
			if req.HTMLBody != "" {
				meta, _ = json.Marshal(map[string]string{"html_body": req.HTMLBody})
			}
			_ = h.enqueueEmail(r.Context(), &req.UserID, email, req.Title, req.Body, "", meta)
		}
	}

	wantTG := req.SendTelegram == nil || *req.SendTelegram
	if wantTG && h.telegram != nil {
		chatID, ok, err := h.store.TelegramChatIDForNotify(r.Context(), req.UserID)
		if err != nil {
			log.Printf("system notify telegram lookup %s: %v", req.UserID, err)
		} else if ok {
			if err := h.telegram.SendText(r.Context(), chatID, req.Title, req.Body); err != nil {
				log.Printf("system notify telegram %s chat %d: %v", req.UserID, chatID, err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
