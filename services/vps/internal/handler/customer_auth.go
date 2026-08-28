package handler

import (
	"net/http"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/authn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (h *Handler) authenticateCustomer(w http.ResponseWriter, r *http.Request, needScopes ...string) (*authn.CustomerIdentity, bool) {
	var pool *pgxpool.Pool
	if h.store != nil {
		pool = h.store.Pool()
	}
	id, err := authn.AuthenticateCustomer(
		r.Context(),
		r.Header.Get("Authorization"),
		r.Header.Get("X-Api-Key"),
		h.jwtSecret,
		h.store,
		pool,
		needScopes...,
	)
	if err != nil {
		status, msg := authn.CustomerAuthErrorStatus(err)
		if status == 0 {
			status = http.StatusUnauthorized
			msg = "unauthorized"
		}
		writeError(w, status, msg)
		return nil, false
	}
	return id, true
}

func (h *Handler) requireCustomer(w http.ResponseWriter, r *http.Request, needScopes ...string) (string, bool) {
	id, ok := h.authenticateCustomer(w, r, needScopes...)
	if !ok || id == nil {
		return "", false
	}
	return id.UserID, true
}