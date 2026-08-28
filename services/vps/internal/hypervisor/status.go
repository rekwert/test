package hypervisor

import "strings"

func IsReadyStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "active", "online", "ready", "complete", "completed", "built", "started":
		return true
	default:
		return false
	}
}

func IsPendingStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "creating", "installing", "pending", "building", "starting", "reinstalling", "queued",
		"building-network", "building-os", "provisioning", "configuring":
		return true
	default:
		return false
	}
}

// ServerOSInstalled reports whether VirtFusion wrote an OS image to the server disk.
// osTemplateInstallId alone is not enough — VF sets it before build finishes.
func ServerOSInstalled(s *Server) bool {
	if s == nil {
		return false
	}
	if s.OSBuilt {
		return true
	}
	if s.BuildFailed && !s.GuestAgentActive {
		return false
	}
	st := strings.ToLower(strings.TrimSpace(s.Status))
	if st == "allocated" || IsPendingStatus(s.Status) {
		return false
	}
	if st == "failed" {
		return s.GuestAgentActive
	}
	return IsReadyStatus(s.Status) || s.GuestAgentActive
}

// ServerReadyForGuestSetup reports whether guest setup can run (password sync, SSH, running).
// VF commission status may read "failed" while the OS image and IP are already present.
func ServerReadyForGuestSetup(s *Server) bool {
	if s == nil || strings.TrimSpace(s.IP) == "" {
		return false
	}
	if !ServerOSInstalled(s) {
		return false
	}
	if s.HasRunningTasks || IsPendingStatus(s.Status) {
		return false
	}
	if IsReadyStatus(s.Status) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(s.Status), "failed")
}

// ServerNeedsBuild reports whether the hypervisor still needs an OS build or finalize step.
func ServerNeedsBuild(s *Server) bool {
	if s == nil || s.OSImageVersionID > 0 || s.HasRunningTasks {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(s.Status), "failed") {
		return true
	}
	if s.OSBuilt && IsReadyStatus(s.Status) {
		if strings.TrimSpace(s.IP) == "" {
			return true
		}
		return false
	}
	return NeedsBuildRetry(s.Status) || IsReadyStatus(s.Status)
}

func NeedsBuildRetry(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "allocated", "active", "awaiting setup", "awaiting_setup", "new", "stopped", "":
		return true
	default:
		return false
	}
}

func IsServerNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, `"404"`) ||
		strings.Contains(msg, "no query results")
}

func DeriveNodeStatus(h ComputeSnapshot) string {
	if h.Maintenance {
		return "maintenance"
	}
	if !h.Enabled {
		return "offline"
	}
	if h.Commissioned != 0 && h.Commissioned != 3 {
		return "warning"
	}
	return "online"
}

func MapServerState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "active", "online", "started", "complete", "completed", "ready", "built":
		return "running"
	case "stopped", "off", "shutdown", "shutoff":
		return "stopped"
	case "starting", "booting":
		return "starting"
	case "restarting", "rebooting":
		return "restarting"
	case "reinstalling", "building-os":
		return "reinstalling"
	case "creating", "installing", "building", "pending", "provisioning":
		return "creating"
	default:
		return ""
	}
}

// ServerPoweredOn reports whether the VM is physically running on the hypervisor.
// VirtFusion "complete" is a commission/build state, not power — guest agent is the reliable signal.
func ServerPoweredOn(s *Server) bool {
	if s == nil {
		return false
	}
	if s.Suspended {
		return false
	}
	if s.RemoteStateKnown && !s.RemoteState {
		return false
	}
	if s.GuestAgentActive {
		return true
	}
	if rs := strings.ToLower(strings.TrimSpace(s.RealStatus)); rs != "" {
		switch rs {
		case "running", "active", "online", "started", "on":
			return true
		case "stopped", "off", "shutdown", "shutoff", "paused":
			return false
		}
	}
	switch strings.ToLower(strings.TrimSpace(s.Status)) {
	case "running", "active", "online", "started", "starting", "booting", "restarting", "rebooting":
		return true
	default:
		return false
	}
}

// MapServerPowerState maps hypervisor server to portal power state for sync/billing.
func MapServerPowerState(s *Server) string {
	if s == nil {
		return ""
	}
	mapped := MapServerState(s.Status)
	if mapped == "creating" || mapped == "reinstalling" ||
		mapped == "starting" || mapped == "restarting" {
		return mapped
	}
	if ServerPoweredOn(s) {
		if mapped == "stopped" {
			return "running"
		}
		if mapped != "" {
			return mapped
		}
		return "running"
	}
	// Commission "complete" with no guest agent means the VM is powered off.
	if mapped == "running" {
		return "stopped"
	}
	return mapped
}
