package catalog

import "testing"

func TestBuildOSCatalogFromImagesWindowsServer(t *testing.T) {
	images := []OSImageVersion{
		{Name: "Windows Server", Version: "2019", VersionID: 21},
		{Name: "Windows Server", Version: "2022", VersionID: 20},
		{Name: "Windows Server", Version: "2025", VersionID: 19},
		{Name: "Windows", Version: "10", VersionID: 22},
		{Name: "Windows", Version: "11", VersionID: 24},
		{Name: "windows11_cloudbase", Version: "", VersionID: 25},
	}
	got := BuildOSCatalogFromImages(images)
	byID := map[string]OSCatalogEntry{}
	for _, row := range got {
		byID[row.ID] = row
	}
	for _, id := range []string{"windows-2019", "windows-2022", "windows-2025", "windows-10", "windows-11"} {
		row, ok := byID[id]
		if !ok {
			t.Fatalf("missing catalog id %q: %+v", id, got)
		}
		if row.Family != "windows" {
			t.Fatalf("%s family = %q", id, row.Family)
		}
		if row.VersionID <= 0 {
			t.Fatalf("%s missing version id", id)
		}
	}
	if byID["windows-10"].Name != "Windows" {
		t.Fatalf("windows-10 name = %q", byID["windows-10"].Name)
	}
	if byID["windows-11"].VersionID != 24 {
		t.Fatalf("windows-11 version id = %d", byID["windows-11"].VersionID)
	}
}

func TestBuildOSCatalogFromImagesDedupesByCatalogID(t *testing.T) {
	images := []OSImageVersion{
		{Name: "Ubuntu", Version: "22.04", VersionID: 12},
		{Name: "Ubuntu", Version: "22.04", VersionID: 99},
	}
	got := BuildOSCatalogFromImages(images)
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d: %+v", len(got), got)
	}
	if got[0].ID != "ubuntu-22.04" || got[0].VersionID != 12 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}
