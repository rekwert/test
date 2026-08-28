package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/auth/internal/clientmeta"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
)

func (h *Handler) AdminListUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil || !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id required")
		return
	}
	keys, err := h.store.ListAPIKeys(r.Context(), userID)
	if err != nil {
		log.Printf("admin list api keys %s: %v", userID, err)
		writeError(w, http.StatusInternalServerError, "failed to list api keys")
		return
	}
	items := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		row := map[string]any{
			"id":         k.ID,
			"name":       k.Name,
			"prefix":     k.KeyPrefix,
			"scopes":     k.Scopes,
			"created_at": k.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if k.LastUsedAt != nil {
			row["last_used_at"] = k.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		items = append(items, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": items})
}

func (h *Handler) AdminCreateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil || !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id required")
		return
	}
	if _, err := h.store.GetUserByID(r.Context(), userID); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "Reseller API"
	}
	if len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name too long")
		return
	}

	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = store.ResellerAPIKeyScopes
	}
	scopes, err = store.NormalizeAPIKeyScopes(scopes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	n, err := h.store.CountActiveAPIKeys(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}
	if n >= store.MaxAPIKeysPerUser {
		writeError(w, http.StatusConflict, "api key limit reached")
		return
	}

	raw, prefix, hash, err := store.NewAPIKeySecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}
	key, err := h.store.CreateAPIKey(r.Context(), userID, req.Name, prefix, hash, scopes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save api key")
		return
	}
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "admin.api_key_create", "api_key", key.ID, map[string]any{
		"target_user_id": userID,
		"name":           key.Name,
		"scopes":         key.Scopes,
	}, clientmeta.Connection(r))

	writeJSON(w, http.StatusCreated, map[string]any{
		"key": map[string]any{
			"id":         key.ID,
			"name":       key.Name,
			"prefix":     key.KeyPrefix,
			"scopes":     key.Scopes,
			"secret":     raw,
			"created_at": key.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		},
		"message": "copy the secret now — it will not be shown again",
	})
}

func (h *Handler) AdminRevokeUserAPIKey(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil || !isStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	userID := strings.TrimSpace(r.PathValue("id"))
	keyID := strings.TrimSpace(r.PathValue("key_id"))
	if userID == "" || keyID == "" {
		writeError(w, http.StatusBadRequest, "user id and key id required")
		return
	}
	if err := h.store.RevokeAPIKey(r.Context(), userID, keyID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "admin.api_key_revoke", "api_key", keyID, map[string]any{
		"target_user_id": userID,
	}, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, messageResponse{Message: "revoked"})
}