package catalog

import (
	"regexp"
	"strings"
)

// OSAlias maps a catalog os_templates.id to hypervisor OS images name + version.
type OSAlias struct {
	CatalogID    string
	ImageName    string
	ImageVersion string
}

// OSAliases is the canonical mapping between portal catalog IDs and hypervisor images.
var OSAliases = []OSAlias{
	{CatalogID: "ubuntu-22.04", ImageName: "Ubuntu", ImageVersion: "22.04"},
	{CatalogID: "ubuntu-24.04", ImageName: "Ubuntu", ImageVersion: "24.04"},
	{CatalogID: "ubuntu-26-04", ImageName: "Ubuntu", ImageVersion: "26.04"},
	{CatalogID: "ubuntu-20.04", ImageName: "Ubuntu", ImageVersion: "20.04"},
	{CatalogID: "ubuntu-18.04", ImageName: "Ubuntu", ImageVersion: "18.04"},
	{CatalogID: "ubuntu-16.04", ImageName: "Ubuntu", ImageVersion: "16.04"},
	{CatalogID: "debian-12", ImageName: "Debian", ImageVersion: "12"},
	{CatalogID: "debian-11", ImageName: "Debian", ImageVersion: "11"},
	{CatalogID: "debian-10", ImageName: "Debian", ImageVersion: "10"},
	{CatalogID: "debian-9", ImageName: "Debian", ImageVersion: "9"},
	{CatalogID: "alma-8", ImageName: "AlmaLinux", ImageVersion: "8.9"},
	{CatalogID: "alma-9", ImageName: "AlmaLinux", ImageVersion: "9.6"},
	{CatalogID: "rocky-8", ImageName: "RockyLinux", ImageVersion: "8.4"},
	{CatalogID: "rocky-9", ImageName: "RockyLinux", ImageVersion: "9"},
	{CatalogID: "centos-9-stream", ImageName: "CentOS", ImageVersion: "9 Stream"},
	{CatalogID: "centos-8-stream", ImageName: "CentOS", ImageVersion: "8 Stream"},
	{CatalogID: "centos-7", ImageName: "CentOS", ImageVersion: "7"},
	{CatalogID: "oracle-8", ImageName: "Oracle Linux", ImageVersion: "8"},
	{CatalogID: "oracle-9", ImageName: "Oracle Linux", ImageVersion: "9"},
	{CatalogID: "astra-ce", ImageName: "Astra Linux", ImageVersion: "CE"},
	{CatalogID: "freebsd-13", ImageName: "FreeBSD", ImageVersion: "13"},
	{CatalogID: "fedora-latest", ImageName: "Fedora", ImageVersion: "Latest"},
	{CatalogID: "opensuse-15", ImageName: "OpenSUSE", ImageVersion: "15"},
	{CatalogID: "opensuse-16", ImageName: "OpenSUSE", ImageVersion: "16"},
	{CatalogID: "alpine-3.15", ImageName: "AlpineLinux", ImageVersion: "3.15"},
	{CatalogID: "windows-2019", ImageName: "windows", ImageVersion: "2019"},
	{CatalogID: "windows-2022", ImageName: "windows", ImageVersion: "2022"},
	{CatalogID: "windows-2022", ImageName: "windows", ImageVersion: "2022 Evaluation"},
	{CatalogID: "windows-2025", ImageName: "windows", ImageVersion: "2025"},
	{CatalogID: "windows-2025", ImageName: "windows", ImageVersion: "2025 Evaluation"},
	{CatalogID: "windows-10", ImageName: "windows", ImageVersion: "10"},
	{CatalogID: "windows-11", ImageName: "windows", ImageVersion: "11"},
	{CatalogID: "windows-11", ImageName: "windows11_cloudbase", ImageVersion: ""},
}

type OSImageVersion struct {
	Name      string
	Version   string
	VersionID int
}

// OSCatalogEntry is one row to upsert into vps.os_templates from the hypervisor.
type OSCatalogEntry struct {
	ID        string
	Name      string
	Version   string
	Family    string
	VersionID int
	SortOrder int
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// BuildOSCatalogFromImages maps every hypervisor image version to a catalog row.
func BuildOSCatalogFromImages(images []OSImageVersion) []OSCatalogEntry {
	aliasByKey := map[string]string{}
	for _, a := range OSAliases {
		key := strings.ToLower(strings.TrimSpace(a.ImageName)) + "|" + strings.ToLower(strings.TrimSpace(a.ImageVersion))
		aliasByKey[key] = a.CatalogID
	}

	seen := map[string]struct{}{}
	out := make([]OSCatalogEntry, 0, len(images))
	order := 0
	for _, img := range images {
		if img.VersionID <= 0 {
			continue
		}
		name := normalizeOSImageName(img.Name)
		key := strings.ToLower(strings.TrimSpace(name)) + "|" + strings.ToLower(strings.TrimSpace(img.Version))
		id, ok := aliasByKey[key]
		if !ok {
			id = slugify(img.Name) + "-" + slugify(img.Version)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		order++
		out = append(out, OSCatalogEntry{
			ID:        id,
			Name:      displayOSName(img.Name),
			Version:   img.Version,
			Family:    osFamily(name),
			VersionID: img.VersionID,
			SortOrder: order,
		})
	}
	return out
}

// MatchOSImages builds catalog_id -> external version_id (legacy helper).
func MatchOSImages(images []OSImageVersion) map[string]int {
	out := make(map[string]int)
	for _, row := range BuildOSCatalogFromImages(images) {
		out[row.ID] = row.VersionID
	}
	return out
}

func displayOSName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "almalinux":
		return "Alma Linux"
	case "alpinelinux":
		return "Alpine Linux"
	case "opensuse":
		return "openSUSE"
	case "windows server":
		return "Windows Server"
	case "windows":
		return "Windows"
	default:
		return strings.TrimSpace(name)
	}
}

func normalizeOSImageName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "windows server"):
		return "windows"
	case n == "windows":
		return "windows"
	default:
		return strings.TrimSpace(name)
	}
}

func osFamily(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "windows"):
		return "windows"
	case strings.Contains(n, "freebsd"):
		return "freebsd"
	case strings.Contains(n, "debian"), strings.Contains(n, "ubuntu"):
		return "debian"
	default:
		return "rhel"
	}
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// CatalogOSMatchesImage reports whether an OS image name/version fits the catalog id.
func CatalogOSMatchesImage(catalogID, imageName, imageVersion string) bool {
	catalogID = strings.TrimSpace(catalogID)
	imageName = strings.ToLower(strings.TrimSpace(imageName))
	imageVersion = strings.ToLower(strings.TrimSpace(imageVersion))
	label := strings.TrimSpace(imageName + " " + imageVersion)
	for _, a := range OSAliases {
		if a.CatalogID == catalogID {
			wantName := strings.ToLower(a.ImageName)
			wantVer := strings.ToLower(a.ImageVersion)
			return strings.Contains(label, wantName) && strings.Contains(label, wantVer)
		}
	}
	slug := strings.ToLower(catalogID)
	if slug == "" {
		return false
	}
	if strings.Contains(label, strings.ReplaceAll(slug, "-", " ")) {
		return true
	}
	return matchSlugTokens(slug, label)
}

func matchSlugTokens(slug, label string) bool {
	for _, tok := range strings.Split(slug, "-") {
		if len(tok) >= 2 && !strings.Contains(label, tok) {
			return false
		}
	}
	return slug != ""
}

func CatalogTemplateByID(id string) (OSTemplate, bool) {
	for _, t := range osTemplates {
		if t.ID == id {
			return t, true
		}
	}
	return OSTemplate{}, false
}

func CleanSoftware() SoftwareProfile {
	return softwareProfiles["clean"]
}

func SoftwareForOSOrClean(osID string) ([]SoftwareProfile, bool) {
	return SoftwareForOS(osID)
}

func SoftwareAllowedOrClean(osID, softwareID string) bool {
	return SoftwareAllowed(osID, softwareID)
}
