package catalog

import "strings"

// Software compatibility by OS family.
// Synced VirtFusion template ids (e.g. debian-11-bullseye) resolve via family,
// not only the static softwareByOS map.
var softwareByFamily = map[string][]string{
	"debian":  {"clean", "3x-ui", "marzban", "python3", "claude-code"},
	"rhel":    {"clean", "3x-ui", "marzban", "python3", "claude-code"},
	"freebsd": {"clean", "python3"},
	"windows": {"clean"},
	"none":    {"clean"},
}

// AmneziaAllowed reports whether Amnezia Docker preinstall is supported on this OS id.
func AmneziaAllowed(osID string) bool {
	osID = strings.TrimSpace(strings.ToLower(osID))
	if osID == "ubuntu-24.04" || osID == "debian-12" {
		return true
	}
	if strings.HasPrefix(osID, "ubuntu-24.04") || strings.Contains(osID, "debian-12") {
		return true
	}
	// VirtFusion synced slugs (e.g. ubuntu-server-24-04-lts-noble-numbat).
	if strings.Contains(osID, "ubuntu") &&
		(strings.Contains(osID, "24-04") || strings.Contains(osID, "24.04") || strings.Contains(osID, "noble")) {
		return true
	}
	return false
}

// claudeCodeBlockedOS rejects EOL images that cannot reliably run Node 20 + Claude Code CLI.
func claudeCodeBlockedOS(osID string) bool {
	osID = strings.TrimSpace(strings.ToLower(osID))
	for _, needle := range []string{
		"centos-7",
		"debian-9",
		"debian-10",
		"ubuntu-16",
		"ubuntu-18",
	} {
		if strings.Contains(osID, needle) {
			return true
		}
	}
	return false
}

// ClaudeCodeAllowed reports whether Claude Code preinstall is supported on this OS id.
func ClaudeCodeAllowed(osID string) bool {
	family := ResolveOSFamily(osID)
	if family != "debian" && family != "rhel" {
		return false
	}
	return !claudeCodeBlockedOS(osID)
}

func filterSoftwareIDs(osID string, ids []string) []string {
	if ClaudeCodeAllowed(osID) {
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "claude-code" {
			continue
		}
		out = append(out, id)
	}
	return out
}

// ResolveOSFamily returns the software family for a catalog / synced OS id.
func ResolveOSFamily(osID string) string {
	osID = strings.TrimSpace(strings.ToLower(osID))
	if osID == "" {
		return "debian"
	}
	if t, ok := CatalogTemplateByID(osID); ok && t.Family != "" {
		return strings.ToLower(t.Family)
	}
	switch {
	case strings.HasPrefix(osID, "windows") || strings.Contains(osID, "windows"):
		return "windows"
	case strings.HasPrefix(osID, "freebsd") || strings.Contains(osID, "freebsd"):
		return "freebsd"
	case strings.HasPrefix(osID, "noos") || osID == "none":
		return "none"
	case strings.HasPrefix(osID, "debian"),
		strings.HasPrefix(osID, "ubuntu"),
		strings.Contains(osID, "debian"),
		strings.Contains(osID, "ubuntu"):
		return "debian"
	case strings.HasPrefix(osID, "alma"),
		strings.HasPrefix(osID, "rocky"),
		strings.HasPrefix(osID, "centos"),
		strings.HasPrefix(osID, "oracle"),
		strings.HasPrefix(osID, "fedora"),
		strings.HasPrefix(osID, "opensuse"),
		strings.HasPrefix(osID, "astra"),
		strings.HasPrefix(osID, "alpine"),
		strings.Contains(osID, "rocky"),
		strings.Contains(osID, "alma"),
		strings.Contains(osID, "centos"),
		strings.Contains(osID, "fedora"),
		strings.Contains(osID, "oracle"),
		strings.Contains(osID, "suse"):
		return "rhel"
	default:
		// Synced linux images we do not recognize yet — allow linux software.
		return "debian"
	}
}

func softwareIDsForOS(osID string) []string {
	osID = strings.TrimSpace(osID)
	var ids []string
	if explicit, ok := softwareByOS[osID]; ok {
		ids = append([]string{}, explicit...)
	} else {
		family := ResolveOSFamily(osID)
		if familyIDs, ok := softwareByFamily[family]; ok {
			ids = append([]string{}, familyIDs...)
		} else {
			ids = []string{"clean"}
		}
	}
	ids = filterSoftwareIDs(osID, ids)
	if ClaudeCodeAllowed(osID) && !containsSoftwareID(ids, "claude-code") {
		ids = append(ids, "claude-code")
	}
	if AmneziaAllowed(osID) && !containsSoftwareID(ids, "amnezia") {
		ids = append(ids, "amnezia")
	}
	return ids
}

func containsSoftwareID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// IsCustom1Plan reports VPS plans bundled with Claude Code only (CUSTOM-1 SKU).
func IsCustom1Plan(planName, planTier string) bool {
	tier := strings.ToLower(strings.TrimSpace(planTier))
	name := strings.ToUpper(strings.TrimSpace(planName))
	if tier == "custom" && (name == "CUSTOM-1" || strings.HasPrefix(name, "CUSTOM-1 ")) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(planName)), "custom-1")
}

// SoftwareAllowedForPlan validates OS + software against plan constraints.
func SoftwareAllowedForPlan(planName, planTier, osID, softwareID string) bool {
	softwareID = strings.TrimSpace(softwareID)
	if softwareID == "" {
		softwareID = "clean"
	}
	if IsCustom1Plan(planName, planTier) {
		if softwareID != "claude-code" {
			return false
		}
		return ClaudeCodeAllowed(osID)
	}
	if !OSAllowedForPlan(planName, planTier, osID) {
		return false
	}
	return SoftwareAllowed(osID, softwareID)
}

// SoftwareProfilesForPlan returns checkout software options for a plan + OS pair.
func SoftwareProfilesForPlan(planName, planTier, osID string) []SoftwareProfile {
	if IsCustom1Plan(planName, planTier) {
		if p, ok := softwareProfiles["claude-code"]; ok && ClaudeCodeAllowed(osID) {
			return []SoftwareProfile{p}
		}
		return nil
	}
	profiles, _ := SoftwareForOS(osID)
	return profiles
}

func EnrichOSTemplatesWithSoftware(templates []OSTemplate) []OSTemplate {
	out := make([]OSTemplate, len(templates))
	for i, t := range templates {
		t.SoftwareIDs = softwareIDsForOS(t.ID)
		out[i] = t
	}
	return out
}

func profilesFromIDs(ids []string) []SoftwareProfile {
	out := make([]SoftwareProfile, 0, len(ids))
	for _, id := range ids {
		if p, ok := softwareProfiles[id]; ok {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []SoftwareProfile{CleanSoftware()}
	}
	return out
}
