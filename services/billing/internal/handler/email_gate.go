package handler

import (
	"net/http"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/authuser"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/authn"
)

func (h *Handler) requireVerifiedCustomer(w http.ResponseWriter, r *http.Request, scopes ...string) (*authn.Identity, bool) {
	id, ok := h.requireCustomer(w, r, scopes...)
	if !ok {
		return nil, false
	}
	if authn.IsStaff(id.Roles) {
		return id, true
	}
	verified, err := authuser.EmailVerified(r.Context(), h.store.Pool(), id.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return nil, false
	}
	if !verified {
		writeError(w, http.StatusForbidden, "email not verified")
		return nil, false
	}
	return id, true
}
