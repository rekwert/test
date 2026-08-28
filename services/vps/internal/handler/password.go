package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

// InstanceChangePassword enqueues an async password change handled by the VPS worker.
func (h *Handler) InstanceChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}

	inst, err := h.store.GetInstanceForUser(r.Context(), userID, instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if store.IsDedicatedProvider(inst.Provider) {
		writeError(w, http.StatusConflict, "password change not supported for dedicated servers")
		return
	}
	if inst.State == "creating" || inst.State == "queued" || inst.State == "reinstalling" || inst.State == "deleted" {
		writeError(w, http.StatusConflict, "server is not ready")
		return
	}
	if inst.IPAddress == nil || *inst.IPAddress == "" {
		writeError(w, http.StatusConflict, "server is still provisioning")
		return
	}

	externalID, err := h.store.GetInstanceExternalID(r.Context(), instanceID)
	if err != nil || externalID == "" {
		writeError(w, http.StatusConflict, "server is still provisioning")
		return
	}

	if t, seen, err := h.store.LastVFPasswordResetAt(r.Context(), instanceID); err == nil && seen {
		if wait := 60*time.Second - time.Since(t); wait > 0 {
			writeError(w, http.StatusConflict, "password change cooldown")
			return
		}
	}

	var req struct {
		Password string `json:"password"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	customPassword := strings.TrimSpace(req.Password)
	if customPassword != "" && len(customPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	if customPassword != "" {
		if err := h.store.SetPendingPasswordChange(r.Context(), instanceID, customPassword); err != nil {
			log.Printf("pending password change %s: %v", instanceID, err)
			writeError(w, http.StatusInternalServerError, "password change failed")
			return
		}
	}

	if err := h.store.EnqueuePasswordChange(r.Context(), instanceID, userID); err != nil {
		if errors.Is(err, store.ErrPasswordChangePending) {
			writeJSON(w, http.StatusAccepted, map[string]any{
				"ok":                      true,
				"pending":                 true,
				"password_change_pending": true,
			})
			return
		}
		log.Printf("enqueue password change %s: %v", instanceID, err)
		writeError(w, http.StatusInternalServerError, "password change failed")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":                      true,
		"pending":                 true,
		"password_change_pending": true,
	})
}
