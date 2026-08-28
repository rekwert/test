package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/auth/internal/clientmeta"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
)

func (h *Handler) AdminGetUserReferrals(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id required")
		return
	}
	if _, err := h.store.GetUserByID(r.Context(), userID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	code, err := h.store.EnsureReferralCode(r.Context(), userID)
	if err != nil {
		log.Printf("admin referrals code %s: %v", userID, err)
		writeError(w, http.StatusInternalServerError, "referral lookup failed")
		return
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("SITE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://cloud-hustle.com"
	}

	referredBy, _ := h.store.GetReferrerForUser(r.Context(), userID)
	rows, err := h.store.ListReferralsByReferrer(r.Context(), userID)
	if err != nil {
		log.Printf("admin referrals list %s: %v", userID, err)
		writeError(w, http.StatusInternalServerError, "referral lookup failed")
		return
	}

	referrals := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		referrals = append(referrals, map[string]any{
			"id":           row.ID,
			"user_id":      row.ReferredID,
			"email":        row.Email,
			"status":       row.Status,
			"total_earned": row.TotalEarned,
			"created_at":   row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	resp := map[string]any{
		"code":      code,
		"link":      baseURL + "/register?ref=" + code,
		"referrals": referrals,
	}
	if referredBy != nil {
		resp["referred_by"] = map[string]any{
			"user_id": referredBy.UserID,
			"email":   referredBy.Email,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) AdminAssignReferral(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	referrerID := strings.TrimSpace(r.PathValue("id"))
	if referrerID == "" {
		writeError(w, http.StatusBadRequest, "user id required")
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}

	if _, err := h.store.GetUserByID(r.Context(), referrerID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	row, err := h.store.AdminAssignReferralByEmail(r.Context(), referrerID, req.Email)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrReferralUserNotFound):
			writeError(w, http.StatusNotFound, "referred user not found")
		case errors.Is(err, store.ErrReferralAlreadyAssigned):
			writeError(w, http.StatusConflict, "user already has a referrer")
		case errors.Is(err, store.ErrReferralSelfAssign):
			writeError(w, http.StatusBadRequest, "cannot refer yourself")
		default:
			log.Printf("admin assign referral %s -> %s: %v", referrerID, req.Email, err)
			writeError(w, http.StatusInternalServerError, "assign failed")
		}
		return
	}

	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "admin.referral.assign", "user", referrerID,
		map[string]any{"referred_user_id": row.ReferredID, "referred_email": row.Email},
		clientmeta.Connection(r))

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"referral": map[string]any{
			"id":           row.ID,
			"user_id":      row.ReferredID,
			"email":        row.Email,
			"status":       row.Status,
			"total_earned": row.TotalEarned,
			"created_at":   row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		},
	})
}
