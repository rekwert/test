package catalog

import "testing"

func TestResolveOSFamily(t *testing.T) {
	cases := map[string]string{
		"ubuntu-24.04":                          "debian",
		"debian-11-bullseye":                    "debian",
		"ubuntu-server-22-04-lts-jammy-jellyfish": "debian",
		"rocky-linux-9":                         "rhel",
		"windows-2022":                          "windows",
		"freebsd-13":                            "freebsd",
	}
	for id, want := range cases {
		if got := ResolveOSFamily(id); got != want {
			t.Fatalf("%s: got %q want %q", id, got, want)
		}
	}
}

func TestEnrichOSTemplatesWithSoftware(t *testing.T) {
	templates := EnrichOSTemplatesWithSoftware([]OSTemplate{
		{ID: "ubuntu-24.04"},
		{ID: "windows-2022"},
	})
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
	if !containsString(templates[0].SoftwareIDs, "3x-ui") {
		t.Fatalf("ubuntu should support 3x-ui: %v", templates[0].SoftwareIDs)
	}
	if containsString(templates[1].SoftwareIDs, "3x-ui") {
		t.Fatalf("windows must not support 3x-ui: %v", templates[1].SoftwareIDs)
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestSoftwareForOSFiltered(t *testing.T) {
	linux, _ := SoftwareForOS("debian-11-bullseye")
	if !hasSoftware(linux, "3x-ui") || !hasSoftware(linux, "marzban") || !hasSoftware(linux, "clean") {
		t.Fatalf("linux should include clean+3x-ui+marzban: %+v", idsOf(linux))
	}
	win, _ := SoftwareForOS("windows-2022")
	if hasSoftware(win, "3x-ui") || !hasSoftware(win, "clean") {
		t.Fatalf("windows should be clean-only: %+v", idsOf(win))
	}
	if !SoftwareAllowed("ubuntu-24.04", "3x-ui") {
		t.Fatal("3x-ui should be allowed on ubuntu")
	}
	if !SoftwareAllowed("ubuntu-24.04", "marzban") {
		t.Fatal("marzban should be allowed on ubuntu")
	}
	if SoftwareAllowed("windows-2019", "3x-ui") {
		t.Fatal("3x-ui must not be allowed on windows")
	}
	if SoftwareAllowed("windows-2019", "marzban") {
		t.Fatal("marzban must not be allowed on windows")
	}
	if !SoftwareAllowed("ubuntu-24.04", "claude-code") {
		t.Fatal("claude-code should be allowed on ubuntu-24.04")
	}
	if SoftwareAllowed("centos-7", "claude-code") {
		t.Fatal("claude-code must not be allowed on centos-7")
	}
	if SoftwareAllowed("windows-2022", "claude-code") {
		t.Fatal("claude-code must not be allowed on windows")
	}
	if !ClaudeCodeAllowed("ubuntu-server-22-04-lts") {
		t.Fatal("claude-code should be allowed on synced ubuntu 22.04")
	}
}

func TestSoftwareAllowedForCustom1Plan(t *testing.T) {
	if !SoftwareAllowedForPlan("CUSTOM-1", "custom", "ubuntu-24.04", "claude-code") {
		t.Fatal("CUSTOM-1 should allow claude-code on ubuntu-24.04")
	}
	if SoftwareAllowedForPlan("CUSTOM-1", "custom", "ubuntu-24.04", "clean") {
		t.Fatal("CUSTOM-1 must not allow clean software")
	}
	if SoftwareAllowedForPlan("CUSTOM-1", "custom", "windows-2022", "claude-code") {
		t.Fatal("CUSTOM-1 must reject windows")
	}
	if SoftwareAllowedForPlan("CUSTOM-1", "custom", "ubuntu-24.04", "3x-ui") {
		t.Fatal("CUSTOM-1 must reject 3x-ui")
	}
	profiles := SoftwareProfilesForPlan("CUSTOM-1", "custom", "ubuntu-24.04")
	if len(profiles) != 1 || profiles[0].ID != "claude-code" {
		t.Fatalf("CUSTOM-1 software list: %+v", profiles)
	}
	if !SoftwareAllowedForPlan("PROSTO-1", "prosto", "ubuntu-24.04", "clean") {
		t.Fatal("PROSTO-1 should still allow clean")
	}
}

func TestAmneziaSoftwareAvailability(t *testing.T) {
	if !SoftwareAllowed("ubuntu-24.04", "amnezia") {
		t.Fatal("amnezia should be allowed on ubuntu-24.04")
	}
	if !SoftwareAllowed("debian-12", "amnezia") {
		t.Fatal("amnezia should be allowed on debian-12")
	}
	if SoftwareAllowed("ubuntu-22.04", "amnezia") {
		t.Fatal("amnezia must not be allowed on ubuntu-22.04")
	}
	if SoftwareAllowed("debian-11", "amnezia") {
		t.Fatal("amnezia must not be allowed on debian-11")
	}
	if !AmneziaAllowed("debian-12-bookworm") {
		t.Fatal("synced debian-12 id should allow amnezia")
	}
	if !AmneziaAllowed("ubuntu-server-24-04-lts-noble-numbat") {
		t.Fatal("synced ubuntu 24.04 id should allow amnezia")
	}
	if !SoftwareAllowed("ubuntu-server-24-04-lts-noble-numbat", "amnezia") {
		t.Fatal("amnezia should be allowed on synced ubuntu 24.04 id")
	}
}

func hasSoftware(list []SoftwareProfile, id string) bool {
	for _, p := range list {
		if p.ID == id {
			return true
		}
	}
	return false
}

func idsOf(list []SoftwareProfile) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.ID)
	}
	return out
}
