package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/abuse"
	"github.com/jackc/pgx/v5"
)

type abuseSignalRequest struct {
	InstanceID string         `json:"instance_id"`
	IP         string         `json:"ip"`
	SignalType string         `json:"signal_type"`
	Weight     int            `json:"weight"`
	Evidence   map[string]any `json:"evidence"`
}

func (h *Handler) InternalAbuseSignal(w http.ResponseWriter, r *http.Request) {
	if !serviceTokenOK(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}

	var req abuseSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.SignalType = strings.TrimSpace(req.SignalType)
	if req.SignalType == "" {
		writeError(w, http.StatusBadRequest, "signal_type required")
		return
	}

	instanceID := strings.TrimSpace(req.InstanceID)
	if instanceID == "" && strings.TrimSpace(req.IP) != "" {
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			writeError(w, http.StatusBadRequest, "valid ip required")
			return
		}
		inst, err := h.store.FindInstanceByIP(r.Context(), ip)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusNotFound, "instance not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		instanceID = inst.ID
	}
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance_id or ip required")
		return
	}

	userID, err := h.store.GetInstanceOwner(r.Context(), instanceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	svc := abuse.NewService(abuse.LoadConfig(), h.store, h.hv())
	stopped, err := svc.RecordSignal(r.Context(), instanceID, userID, abuse.Signal{
		Type:     req.SignalType,
		Weight:   0, // always use server-side weights; ignore client-supplied weight
		Evidence: req.Evidence,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signal processing failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"auto_stopped": stopped,
		"instance_id": instanceID,
	})
}

func (h *Handler) AdminAbuseFalsePositive(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.staffOnly(w, r)
	if !ok {
		return
	}
	caseID := strings.TrimSpace(r.PathValue("id"))
	if caseID == "" {
		writeError(w, http.StatusBadRequest, "case id required")
		return
	}
	if err := h.store.ResolveAbuseCaseFalsePositive(r.Context(), caseID, claims.UserID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func serviceTokenOK(r *http.Request) bool {
	got := strings.TrimSpace(r.Header.Get("X-Service-Token"))
	if got == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			got = strings.TrimSpace(auth[7:])
		}
	}
	want := strings.TrimSpace(os.Getenv("ABUSE_INGEST_TOKEN"))
	return want != "" && got == want
}
