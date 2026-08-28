package catalog

import "testing"

func TestIsWindowsClient10Or11(t *testing.T) {
	yes := []string{"windows-10", "windows-11", "windows11_cloudbase", "Windows-10", "synced-windows-11-lts"}
	no := []string{
		"windows-2019", "windows-2022", "windows-2025",
		"ubuntu-24.04", "debian-12", "windows-server-2022",
	}
	for _, id := range yes {
		if !IsWindowsClient10Or11(id) {
			t.Fatalf("%q should be detected as Windows 10/11 client", id)
		}
	}
	for _, id := range no {
		if IsWindowsClient10Or11(id) {
			t.Fatalf("%q should not be treated as Windows 10/11 client", id)
		}
	}
}

func TestSoftwareAllowedForProsto1Plan(t *testing.T) {
	if !SoftwareAllowedForPlan("PROSTO-1", "prosto", "ubuntu-24.04", "clean") {
		t.Fatal("PROSTO-1 should allow ubuntu")
	}
	if SoftwareAllowedForPlan("PROSTO-1", "prosto", "windows-10", "clean") {
		t.Fatal("PROSTO-1 must reject windows-10")
	}
	if SoftwareAllowedForPlan("PROSTO-1", "prosto", "windows-11", "clean") {
		t.Fatal("PROSTO-1 must reject windows-11")
	}
	if SoftwareAllowedForPlan("PROSTO-1", "prosto", "windows-2022", "clean") {
		t.Fatal("PROSTO-1 must reject windows server")
	}
	if SoftwareAllowedForPlan("PROSTO-1", "prosto", "windows-2025", "clean") {
		t.Fatal("PROSTO-1 must reject windows-2025")
	}
	if !SoftwareAllowedForPlan("PROSTO-2", "prosto", "windows-11", "clean") {
		t.Fatal("PROSTO-2 should allow windows-11")
	}
	if !SoftwareAllowedForPlan("PROSTO-2", "prosto", "windows-2025", "clean") {
		t.Fatal("PROSTO-2 should allow windows-2025")
	}
}
