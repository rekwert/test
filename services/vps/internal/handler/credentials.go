package handler

import (
	"net/http"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
)

func (h *Handler) InstanceCredentials(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireVerifiedCustomer(w, r, "vps.read")
	if !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	if _, err := h.store.GetInstanceForUser(r.Context(), userID, instanceID); err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if !h.requireBillingAccess(w, r, instanceID) {
		return
	}

	creds, err := h.store.GetInstanceCredentials(r.Context(), userID, instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if creds.State != "running" {
		writeError(w, http.StatusConflict, "credentials available when server is running")
		return
	}
	if creds.IPAddress == "" {
		writeError(w, http.StatusConflict, "ip address not assigned yet")
		return
	}
	if strings.TrimSpace(creds.RootPassword) == "" {
		writeError(w, http.StatusConflict, "root password not available yet")
		return
	}

	hostname := strings.TrimSpace(creds.Hostname)
	if hostname == "" {
		hostname = "vps"
	}
	osTemplateID, _ := h.store.GetInstanceOSTemplateID(r.Context(), instanceID)
	loginUser := catalog.ResolvePasswordResetUser(osTemplateID)

	pending, _ := h.store.HasPendingPasswordChange(r.Context(), instanceID)

	writeJSON(w, http.StatusOK, map[string]any{
		"hostname":                hostname,
		"ip_address":              creds.IPAddress,
		"all_ips":                 creds.AllIPs,
		"username":                loginUser,
		"root_password":           creds.RootPassword,
		"ssh_port":                22,
		"password_change_pending": pending,
	})
}
