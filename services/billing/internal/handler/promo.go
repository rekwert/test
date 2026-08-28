package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/borishru-boop/testVPStrade/services/billing/internal/store"
)

func (h *Handler) ValidatePromo(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireVerifiedCustomer(w, r, "billing")
	if !ok {
		return
	}

	var req struct {
		Code   string  `json:"code"`
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}

	preview, err := h.store.PreviewPromo(r.Context(), id.UserID, req.Code, "topup", req.Amount)
	if err != nil {
		writePromoError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, promoPreviewJSON(preview))
}

func (h *Handler) ApplyPromo(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireVerifiedCustomer(w, r, "billing")
	if !ok {
		return
	}

	var req struct {
		Code       string  `json:"code"`
		Amount     float64 `json:"amount"`
		InstanceID string  `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}

	preview, err := h.store.PreviewPromo(r.Context(), id.UserID, req.Code, "topup", req.Amount)
	if err != nil {
		writePromoError(w, err)
		return
	}

	switch preview.Kind {
	case "credit":
		credited, err := h.store.ApplyCreditPromo(r.Context(), id.UserID, req.Code)
		if err != nil {
			writePromoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"kind":          preview.Kind,
			"credit_amount": credited,
			"message":       preview.Description,
		})
	case "topup_bonus_percent", "topup_bonus_fixed":
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":           true,
			"kind":         preview.Kind,
			"bonus_amount": preview.BonusAmount,
			"code":         preview.Code,
			"message":      preview.Description,
		})
	case "charge_discount_percent":
		var instID *string
		if req.InstanceID != "" {
			instID = &req.InstanceID
		}
		applied, err := h.store.ApplyChargeDiscountPromo(r.Context(), id.UserID, req.Code, instID)
		if err != nil {
			writePromoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"kind":             applied.Kind,
			"discount_percent": applied.DiscountPercent,
			"message":          applied.Description,
		})
	default:
		writeError(w, http.StatusBadRequest, "unsupported promo type")
	}
}

func promoPreviewJSON(p *store.PromoPreview) map[string]any {
	if p == nil {
		return map[string]any{}
	}
	return map[string]any{
		"code":             p.Code,
		"kind":             p.Kind,
		"bonus_amount":     p.BonusAmount,
		"discount_percent": p.DiscountPercent,
		"description":      p.Description,
	}
}

func writePromoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPromoNotFound):
		writeError(w, http.StatusNotFound, "promo not found")
	case errors.Is(err, store.ErrPromoAlreadyUsed):
		writeError(w, http.StatusConflict, "promo already used")
	case errors.Is(err, store.ErrPromoInvalid):
		writeError(w, http.StatusBadRequest, "promo invalid or expired")
	default:
		log.Printf("promo error: %v", err)
		writeError(w, http.StatusInternalServerError, "promo failed")
	}
}

func (h *Handler) resolveTopupPromo(w http.ResponseWriter, r *http.Request, userID, code string, amount float64) (*store.PromoPreview, bool) {
	if code == "" {
		return nil, true
	}
	preview, err := h.store.ResolveTopupPromo(r.Context(), userID, code, amount)
	if err != nil {
		writePromoError(w, err)
		return nil, false
	}
	return preview, true
}

func promoIDPtr(preview *store.PromoPreview) *string {
	if preview == nil {
		return nil
	}
	id := preview.PromoID
	return &id
}

func promoBonus(preview *store.PromoPreview) float64 {
	if preview == nil {
		return 0
	}
	return preview.BonusAmount
}
