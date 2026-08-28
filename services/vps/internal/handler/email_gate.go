package handler

import (
	"net/http"
)

func (h *Handler) requireVerifiedCustomer(w http.ResponseWriter, r *http.Request, needScopes ...string) (userID string, ok bool) {
	id, ok := h.authenticateCustomer(w, r, needScopes...)
	if !ok || id == nil {
		return "", false
	}
	return id.UserID, true
}
