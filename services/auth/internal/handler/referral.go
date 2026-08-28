package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
)

func (h *Handler) ReferralDashboard(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	dashboard, err := h.store.GetReferralDashboard(r.Context(), userID, h.cfg.ReferralBaseURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load referral data")
		return
	}

	events := make([]map[string]any, 0, len(dashboard.Events))
	for _, ev := range dashboard.Events {
		events = append(events, map[string]any{
			"id":            ev.ID,
			"masked_email":  ev.MaskedEmail,
			"status":        ev.Status,
			"earned":        ev.Earned,
			"created_at":    ev.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code": dashboard.Code,
		"link": dashboard.Link,
		"program": map[string]any{
			"referrer_percent": store.ReferrerPercent,
			"currency":         store.ReferralCurrency,
		},
		"stats": map[string]any{
			"total_earned":       dashboard.Stats.TotalEarned,
			"pending_earned":     dashboard.Stats.PendingEarned,
			"earned_this_month":  dashboard.Stats.EarnedThisMonth,
			"active_referrals":   dashboard.Stats.ActiveReferrals,
			"pending_referrals":  dashboard.Stats.PendingReferrals,
			"link_clicks":        dashboard.Stats.LinkClicks,
		},
		"hold_days": 30,
		"events": events,
	})
}

func (h *Handler) ReferralClick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}
	if err := h.store.RecordReferralClick(r.Context(), code); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record click")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
