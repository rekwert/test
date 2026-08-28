package catalog

import "strings"

// IsProsto1Plan reports the entry PROSTO-1 SKU (1 vCPU / 1 GB / 10 GB).
func IsProsto1Plan(planName, planTier string) bool {
	name := strings.ToUpper(strings.TrimSpace(planName))
	if name == "PROSTO-1" || strings.HasPrefix(name, "PROSTO-1 ") {
		return true
	}
	return false
}

// IsWindowsClient10Or11 reports desktop Windows 10/11 images (not Windows Server).
// Kept for callers that still need the client/desktop distinction.
func IsWindowsClient10Or11(osID string) bool {
	osID = strings.ToLower(strings.TrimSpace(osID))
	if osID == "" {
		return false
	}
	switch osID {
	case "windows-10", "windows-11":
		return true
	}
	if strings.Contains(osID, "server") {
		return false
	}
	if strings.Contains(osID, "windows11") || strings.Contains(osID, "windows-11") {
		return true
	}
	if strings.Contains(osID, "windows-10") || strings.Contains(osID, "windows10") {
		return true
	}
	if t, ok := CatalogTemplateByID(osID); ok {
		return isWindowsClient10Or11Template(t)
	}
	return false
}

func isWindowsClient10Or11Template(t OSTemplate) bool {
	if strings.ToLower(strings.TrimSpace(t.Family)) != "windows" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(t.Name))
	if strings.Contains(name, "server") {
		return false
	}
	v := strings.TrimSpace(t.Version)
	return v == "10" || v == "11"
}

// OSAllowedForPlan validates whether an OS template may be used on a plan SKU.
// PROSTO-1 (1/1/10) cannot run any Windows image — desktop or Server.
func OSAllowedForPlan(planName, planTier, osID string) bool {
	if IsProsto1Plan(planName, planTier) && IsWindowsOS(osID) {
		return false
	}
	return true
}
