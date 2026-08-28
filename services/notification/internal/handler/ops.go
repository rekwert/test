package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// OpsAlert posts a message to the ops/alerts Telegram channel
// (same chat as cloud-hustle-monitoring daily summaries / Alertmanager).
func (h *Handler) OpsAlert(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.Header.Get("X-Service-Token"))
	expected := os.Getenv("NOTIFICATION_SERVICE_TOKEN")
	if expected == "" || token != expected {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" && req.Title == "" {
		writeError(w, http.StatusBadRequest, "title or body required")
		return
	}
	if req.Title == "" {
		req.Title = "Ops alert"
	}

	if h.alerts == nil || h.alertsChatID == 0 {
		log.Printf("ops alert skipped (telegram alerts not configured): %s", req.Title)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": false, "reason": "alerts_not_configured"})
		return
	}

	if err := h.alerts.SendText(r.Context(), h.alertsChatID, req.Title, req.Body); err != nil {
		log.Printf("ops alert telegram: %v", err)
		writeError(w, http.StatusBadGateway, "telegram send failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": true})
}
