package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/auth/internal/clientmeta"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
)

func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	q := r.URL.Query().Get("q")
	role := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("role")))
	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	emailVerified := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("email_verified")))
	if role != "" && !validAdminRoleFilter(role) {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}
	if status != "" && !validBillingStatusFilter(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if emailVerified != "" && emailVerified != "true" && emailVerified != "false" {
		writeError(w, http.StatusBadRequest, "invalid email_verified")
		return
	}
	createdFrom := r.URL.Query().Get("created_from")
	createdTo := r.URL.Query().Get("created_to")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	items, total, err := h.store.ListUsers(r.Context(), store.UserListFilters{
		Query:         q,
		Role:          role,
		Status:        status,
		EmailVerified: emailVerified,
		CreatedFrom:   createdFrom,
		CreatedTo:     createdTo,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		if errors.Is(err, store.ErrInvalidRoleFilter) ||
			errors.Is(err, store.ErrInvalidStatusFilter) ||
			errors.Is(err, store.ErrInvalidEmailVerifiedFilter) {
			writeError(w, http.StatusBadRequest, "invalid filter")
			return
		}
		log.Printf("admin list users: %v", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}

	users := make([]map[string]any, 0, len(items))
	for _, u := range items {
		row := map[string]any{
			"id":             u.ID,
			"email":          u.Email,
			"display_name":   u.DisplayName,
			"email_verified": u.EmailVerified,
			"roles":          u.Roles,
			"billing_status": u.BillingStatus,
			"created_at":     u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if u.LastActiveAt != nil {
			row["last_active_at"] = u.LastActiveAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		users = append(users, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":  users,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) AdminGetUser(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.PathValue("id")
	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	billingStatus, _ := h.store.GetUserBillingStatus(r.Context(), userID)
	activity, _ := h.store.GetUserActivityTimes(r.Context(), userID)

	row := map[string]any{
		"id":             user.ID,
		"email":          user.Email,
		"display_name":   user.DisplayName,
		"email_verified": user.EmailVerified,
		"locale":         user.Locale,
		"roles":          user.Roles,
		"billing_status": billingStatus,
		"created_at":     user.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if activity != nil {
		if portalAt := activity.PortalAt(); portalAt != nil {
			row["last_active_at"] = portalAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		if activity.LastLoginAt != nil {
			row["last_login_at"] = activity.LastLoginAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": row})
}

func isStaff(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "owner", "admin", "support":
			return true
		}
	}
	return false
}

func hasRole(roles []string, name string) bool {
	for _, r := range roles {
		if r == name {
			return true
		}
	}
	return false
}

func (h *Handler) AdminListAudit(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.URL.Query().Get("user_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	items, total, err := h.store.ListAuditLog(r.Context(), store.AuditListFilters{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}

	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		row := map[string]any{
			"id":         e.ID,
			"action":     e.Action,
			"entity":     e.Entity,
			"created_at": e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if e.ActorID != nil {
			row["actor_id"] = *e.ActorID
		}
		if e.EntityID != nil {
			row["entity_id"] = *e.EntityID
		}
		if e.Metadata != nil {
			row["metadata"] = e.Metadata
			row["details"] = e.Metadata
		}
		if e.IP != nil {
			row["ip"] = *e.IP
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) AdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	userID := r.PathValue("id")
	if userID == claims.UserID {
		writeError(w, http.StatusBadRequest, "cannot delete yourself")
		return
	}

	deletedEmail, err := h.store.SoftDeleteUser(r.Context(), userID, claims.UserID)
	if err != nil {
		switch {
		case store.IsNotFound(err):
			writeError(w, http.StatusNotFound, "user not found")
		case errors.Is(err, store.ErrUserDeleted):
			writeError(w, http.StatusConflict, "user already deleted")
		case errors.Is(err, store.ErrCannotDeleteStaff):
			writeError(w, http.StatusForbidden, "cannot delete staff user")
		default:
			log.Printf("admin delete user: %v", err)
			writeError(w, http.StatusInternalServerError, "delete failed")
		}
		return
	}

	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "admin.user.delete", "user", userID,
		map[string]any{"email": deletedEmail}, clientmeta.Connection(r))
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "auth.sessions.revoke_all", "user", userID,
		map[string]any{"reason": "account_deleted"}, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validAdminRoleFilter(role string) bool {
	switch role {
	case "client", "support", "admin", "owner":
		return true
	default:
		return false
	}
}

func validBillingStatusFilter(status string) bool {
	switch status {
	case "active", "suspended", "past_due", "grace_period":
		return true
	default:
		return false
	}
}

func (h *Handler) AdminSetUserRoles(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasRole(claims.Roles, "owner") {
		writeError(w, http.StatusForbidden, "only owner can manage staff roles")
		return
	}

	userID := r.PathValue("id")
	var req struct {
		Roles []string `json:"roles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	roles, err := h.store.SetUserStaffRoles(r.Context(), claims.Roles, userID, req.Roles)
	if err != nil {
		switch {
		case store.IsNotFound(err):
			writeError(w, http.StatusNotFound, "user not found")
		case errors.Is(err, store.ErrInvalidStaffRole):
			writeError(w, http.StatusBadRequest, "invalid role")
		case errors.Is(err, store.ErrOnlyOwnerCanManageRoles):
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, store.ErrCannotAssignOwner), errors.Is(err, store.ErrCannotAssignAdmin):
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, store.ErrCannotModifyOwner):
			writeError(w, http.StatusForbidden, "cannot modify owner")
		default:
			log.Printf("admin set roles: %v", err)
			writeError(w, http.StatusInternalServerError, "update failed")
		}
		return
	}

	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "admin.user.roles", "user", userID,
		map[string]any{"roles": roles}, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "roles": roles})
}

func (h *Handler) AdminImpersonateUser(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !isAdminOrOwner(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	targetID := r.PathValue("id")
	if targetID == claims.UserID {
		writeError(w, http.StatusBadRequest, "cannot impersonate yourself")
		return
	}

	email, roles, err := h.store.GetUserAuthProfile(r.Context(), targetID)
	if err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	for _, role := range roles {
		if role == "owner" || role == "admin" {
			if !isAdminOrOwner(claims.Roles) {
				writeError(w, http.StatusForbidden, "cannot impersonate staff")
				return
			}
		}
	}

	access, exp, err := h.tokens.IssueAccessImpersonation(claims.UserID, targetID, email, roles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token failed")
		return
	}

	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "admin.user.impersonate", "user", targetID, nil, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":     access,
		"expires_at":       exp.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"impersonated_by":  claims.UserID,
		"user": map[string]any{
			"id":    targetID,
			"email": email,
			"roles": roles,
		},
	})
}

func isAdminOrOwner(roles []string) bool {
	for _, r := range roles {
		if r == "owner" || r == "admin" {
			return true
		}
	}
	return false
}
