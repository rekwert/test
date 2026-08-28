package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) AdminExtendInstanceBilling(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.staffOnly(w, r)
	if !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	var req struct {
		ExtendDays          int    `json:"extend_days"`
		BillingPeriodDays   *int   `json:"billing_period_days"`
		Reason              string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	detail, err := h.store.ExtendInstanceBilling(r.Context(), instanceID, req.ExtendDays, req.BillingPeriodDays, claims.UserID, strings.TrimSpace(req.Reason))
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "extend failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "instance": instanceDetailToJSON(detail)})
}

func (h *Handler) AdminTransferClient(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	fromUserID := r.PathValue("user_id")
	if fromUserID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	var req struct {
		ToUserID        string `json:"to_user_id"`
		ToEmail         string `json:"to_email"`
		TransferBalance bool   `json:"transfer_balance"`
		Reason          string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.ToUserID = strings.TrimSpace(req.ToUserID)
	req.ToEmail = strings.TrimSpace(req.ToEmail)
	if req.ToUserID == "" && req.ToEmail == "" {
		writeError(w, http.StatusBadRequest, "to_email or to_user_id required")
		return
	}
	if req.ToUserID == "" {
		resolved, err := h.store.ResolveUserIDByEmail(r.Context(), req.ToEmail)
		if err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				writeError(w, http.StatusBadRequest, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "resolve user failed")
			return
		}
		req.ToUserID = resolved
	}
	result, err := h.store.TransferClient(r.Context(), fromUserID, req.ToUserID, claims.UserID, req.TransferBalance, strings.TrimSpace(req.Reason))
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusBadRequest, "user not found")
			return
		}
		if strings.Contains(err.Error(), "same user") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "transfer failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) AdminInstanceDiagnostics(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}

	detail, err := h.store.GetInstanceDetail(r.Context(), instanceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	checkPort := r.URL.Query().Get("check_port") != "false"
	resp := map[string]any{
		"instance": instanceDetailToJSON(detail),
		"checks":   []map[string]any{},
		"summary":  "ok",
	}
	checks := make([]map[string]any, 0, 12)

	checks = append(checks, map[string]any{
		"id": "vm_state", "ok": detail.State == "running", "label": "vm_state",
		"detail": detail.State,
	})
	checks = append(checks, map[string]any{
		"id": "billing_status", "ok": detail.BillingStatus == "active", "label": "billing_status",
		"detail": detail.BillingStatus,
	})

	if detail.NodeID != nil && *detail.NodeID != "" {
		nodeOK := detail.NodeStatus != nil && *detail.NodeStatus == "online"
		nodeDetail := map[string]any{
			"name":   derefStr(detail.NodeName),
			"status": derefStr(detail.NodeStatus),
		}
		if nodeStats, err := h.store.GetNodeStats(r.Context(), *detail.NodeID); err == nil && nodeStats != nil {
			nodeOK = nodeStats.Status == "online" &&
				(nodeStats.VFEnabled == nil || *nodeStats.VFEnabled) &&
				!nodeStats.MaintenanceMode
			nodePayload := map[string]any{
				"name":             nodeStats.Name,
				"status":           nodeStats.Status,
				"region":           nodeStats.Region,
				"uptime":           nodeStats.Uptime,
				"load_percent":     nodeStats.LoadPercent,
				"instance_count":   nodeStats.InstanceCount,
				"capacity":         nodeStats.Capacity,
				"maintenance_mode": nodeStats.MaintenanceMode,
			}
			if nodeStats.VFEnabled != nil {
				nodePayload["vf_enabled"] = *nodeStats.VFEnabled
			}
			if nodeStats.VFServerCount != nil {
				nodePayload["vf_server_count"] = *nodeStats.VFServerCount
			}
			if nodeStats.LastSyncedAt != nil {
				nodePayload["last_synced_at"] = nodeStats.LastSyncedAt.UTC().Format(time.RFC3339)
			}
			resp["node"] = nodePayload
			nodeDetail["uptime"] = nodeStats.Uptime
			nodeDetail["load_percent"] = nodeStats.LoadPercent
		}
		checks = append(checks, map[string]any{
			"id": "node", "ok": nodeOK, "label": "node", "detail": nodeDetail,
		})
	}

	isWindows := catalog.IsWindowsOS(detail.OSTemplateID)

	externalID := instanceID
	if detail.ExternalID != nil && *detail.ExternalID != "" {
		externalID = *detail.ExternalID
	}
	vfServer, vfErr := h.hv().GetServer(r.Context(), externalID)
	hvReady := vfServer != nil && hypervisor.IsReadyStatus(vfServer.Status)
	if vfErr == nil && vfServer != nil {
		checks = append(checks, map[string]any{
			"id": "hypervisor", "ok": hvReady, "label": "hypervisor",
			"detail": vfServer.Status,
		})
	}

	metrics, metricsErr := h.hv().GetMetrics(r.Context(), externalID)
	guestOK := metricsErr == nil && metrics != nil
	guestDetail := "unavailable"
	if guestOK {
		guestDetail = "ok"
	} else if !isWindows {
		guestOK = true
		guestDetail = "optional_unavailable"
	}
	checks = append(checks, map[string]any{
		"id": "guest_agent", "ok": guestOK, "label": "guest_agent", "detail": guestDetail,
	})
	if metrics != nil {
		resp["metrics"] = metrics
	}

	failedTasks, _ := h.store.CountFailedOutbox(r.Context(), instanceID)
	checks = append(checks, map[string]any{
		"id": "failed_tasks", "ok": failedTasks == 0, "label": "failed_tasks",
		"detail": failedTasks,
	})

	ip := ""
	if detail.IPAddress != nil {
		ip = *detail.IPAddress
	}
	if ip == "" && vfServer != nil {
		ip = vfServer.IP
	}
	sshReachable := false
	var ssh map[string]any
	var rdp map[string]any
	if checkPort && ip != "" {
		ssh = probePort(ip, "22")
		sshReachable = ssh["reachable"] == true
		checks = append(checks, map[string]any{
			"id": "port_22", "ok": sshReachable, "label": "port_22", "detail": ssh,
		})
		if isWindows {
			rdp = probePort(ip, "3389")
			checks = append(checks, map[string]any{
				"id": "port_3389", "ok": rdp["reachable"] == true, "label": "port_3389", "detail": rdp,
			})
		} else {
			rdp = map[string]any{"reachable": false, "applicable": false}
			checks = append(checks, map[string]any{
				"id": "port_3389", "ok": true, "label": "port_3389", "detail": rdp,
			})
		}
		resp["port_checks"] = map[string]any{
			"ip": ip, "ssh_22": ssh, "rdp_3389": rdp, "is_windows": isWindows,
		}
	}

	vmRunning := detail.State == "running"
	silentBlock := vmRunning && hvReady && ip != "" && checkPort && !sshReachable
	checks = append(checks, map[string]any{
		"id": "silent_block", "ok": !silentBlock, "label": "silent_block",
		"detail": map[string]any{
			"suspected": silentBlock,
			"vm_running": vmRunning,
			"port_open": sshReachable,
		},
	})

	uptime := ""
	if detail.CreatedAt.Before(time.Now()) {
		uptime = formatDuration(time.Since(detail.CreatedAt))
	}
	resp["vm"] = map[string]any{
		"state":      detail.State,
		"ip":         ip,
		"uptime":     uptime,
		"region":     detail.Region,
		"plan":       detail.PlanName,
		"paid_until": formatTimePtr(detail.NextBillingAt),
	}

	actions, _ := h.store.ListAdminActions(r.Context(), "", instanceID, 8)
	actionOut := make([]map[string]any, 0, len(actions))
	for _, a := range actions {
		actionOut = append(actionOut, map[string]any{
			"id": a.ID, "action": a.Action, "created_at": a.CreatedAt.UTC().Format(time.RFC3339),
			"details": json.RawMessage(a.Details),
		})
	}
	resp["recent_actions"] = actionOut
	resp["checks"] = checks

	hasIssues := false
	for _, c := range checks {
		if ok, _ := c["ok"].(bool); !ok {
			hasIssues = true
			break
		}
	}
	if hasIssues {
		resp["summary"] = "issues"
	}
	writeJSON(w, http.StatusOK, resp)
}

func instanceDetailToJSON(detail *store.InstanceDetail) map[string]any {
	row := map[string]any{
		"id":                  detail.ID,
		"user_id":             detail.UserID,
		"state":               detail.State,
		"region":              detail.Region,
		"plan_id":             detail.PlanID,
		"plan_name":           detail.PlanName,
		"billing_status":      detail.BillingStatus,
		"billing_period_days": detail.BillingPeriodDays,
	}
	if detail.Hostname != nil && *detail.Hostname != "" {
		row["hostname"] = *detail.Hostname
	}
	if detail.IPAddress != nil && *detail.IPAddress != "" {
		row["ip_address"] = *detail.IPAddress
	}
	if detail.NodeID != nil {
		row["node_id"] = *detail.NodeID
	}
	if detail.NodeName != nil {
		row["node_name"] = *detail.NodeName
	}
	if detail.NextBillingAt != nil {
		row["next_billing_at"] = detail.NextBillingAt.UTC().Format(time.RFC3339)
	}
	return row
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h"
	}
	if hours > 0 {
		return strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
	}
	return strconv.Itoa(mins) + "m"
}
