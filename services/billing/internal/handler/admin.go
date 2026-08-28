package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/borishru-boop/testVPStrade/services/billing/internal/authn"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/store"
)

func (h *Handler) AdminUserBalance(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.PathValue("id")
	acc, status, err := h.store.GetBalanceWithStatus(r.Context(), userID)
	if err != nil {
		log.Printf("admin balance: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load balance")
		return
	}
	totalTopups, err := h.store.SumUserTopups(r.Context(), userID)
	if err != nil {
		log.Printf("admin balance topups sum: %v", err)
		totalTopups = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":         acc.UserID,
		"balance":         acc.Balance,
		"currency":        acc.Currency,
		"billing_status":  status,
		"total_topups":    totalTopups,
	})
}

func (h *Handler) AdminUserInvoices(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.PathValue("id")
	items, err := h.store.ListInvoices(r.Context(), userID)
	if err != nil {
		log.Printf("admin invoices: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load invoices")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, inv := range items {
		row := invoiceToJSON(inv)
		// admin list uses RFC3339 with offset for consistency with other admin APIs
		row["created_at"] = inv.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoices": out})
}

func (h *Handler) AdminRefund(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsAdminOrOwner(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.PathValue("id")
	var req struct {
		Amount float64 `json:"amount"`
		Reason string  `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	newBalance, err := h.store.AdminRefund(r.Context(), userID, req.Amount, req.Reason, claims.UserID)
	if err != nil {
		if err == store.ErrInsufficientBalance {
			writeError(w, http.StatusBadRequest, "insufficient balance")
			return
		}
		log.Printf("admin refund: %v", err)
		writeError(w, http.StatusInternalServerError, "refund failed")
		return
	}
	_ = h.store.InsertAuthAudit(r.Context(), claims.UserID, "admin.refund", "user", userID, map[string]any{
		"amount": req.Amount,
		"reason": req.Reason,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"balance":  newBalance,
		"currency": "RUB",
	})
}

func (h *Handler) AdminCredit(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsAdminOrOwner(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.PathValue("id")
	var req struct {
		Amount float64 `json:"amount"`
		Reason string  `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	newBalance, err := h.store.AdminCredit(r.Context(), userID, req.Amount, req.Reason, claims.UserID)
	if err != nil {
		log.Printf("admin credit: %v", err)
		writeError(w, http.StatusInternalServerError, "credit failed")
		return
	}
	_ = h.store.InsertAuthAudit(r.Context(), claims.UserID, "admin.credit", "user", userID, map[string]any{
		"amount": req.Amount,
		"reason": req.Reason,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"balance":  newBalance,
		"currency": "RUB",
	})
}

func (h *Handler) AdminAdjustments(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListAdjustments(r.Context(), userID, limit)
	if err != nil {
		log.Printf("admin adjustments: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load adjustments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"adjustments": items})
}

func (h *Handler) AdminBusinessStats(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	period := store.NewStatsPeriod(days, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	stats, err := h.store.BusinessStats(r.Context(), period)
	if err != nil {
		log.Printf("admin business stats: %v", err)
		writeError(w, http.StatusInternalServerError, "stats failed")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) AdminListTopups(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	period := store.NewStatsPeriod(days, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.store.ListPaidTopups(r.Context(), period, limit, offset)
	if err != nil {
		log.Printf("admin topups: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load topups")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, inv := range items {
		row := map[string]any{
			"id":         inv.ID,
			"user_id":    inv.UserID,
			"user_email": inv.UserEmail,
			"amount":     inv.Amount,
			"currency":   inv.Currency,
			"status":     inv.Status,
			"type":       "topup",
			"created_at": inv.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if inv.Provider != nil {
			row["provider"] = *inv.Provider
		}
		if inv.Description != nil {
			row["description"] = *inv.Description
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"topups":      out,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
		"period_from": period.From.Format("2006-01-02"),
		"period_to":   period.To.Format("2006-01-02"),
	})
}

func (h *Handler) AdminLedger(w http.ResponseWriter, r *http.Request) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.secret)
	if err != nil || !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	userID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListLedger(r.Context(), userID, limit)
	if err != nil {
		log.Printf("admin ledger: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load ledger")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		row := map[string]any{
			"id":         e.ID,
			"kind":       e.Kind,
			"amount":     e.Amount,
			"direction":  e.Direction,
			"status":     e.Status,
			"provider":   e.Provider,
			"description": e.Description,
			"created_at": e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if e.InstanceID != "" {
			row["instance_id"] = e.InstanceID
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}
