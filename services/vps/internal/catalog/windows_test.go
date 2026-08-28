package catalog

import "testing"

func TestResolvePasswordResetUser(t *testing.T) {
	tests := []struct {
		osID string
		want string
	}{
		{"ubuntu-22.04", "root"},
		{"debian-12", "root"},
		{"windows-10", "Administrator"},
		{"windows-11", "Administrator"},
		{"windows-2022", "Administrator"},
		{"Windows 10", "Administrator"},
	}
	for _, tc := range tests {
		if got := ResolvePasswordResetUser(tc.osID); got != tc.want {
			t.Fatalf("ResolvePasswordResetUser(%q) = %q, want %q", tc.osID, got, tc.want)
		}
	}
}

func TestIsWindowsOS(t *testing.T) {
	if !IsWindowsOS("windows-10") {
		t.Fatal("windows-10")
	}
	if !IsWindowsOS("windows-11") {
		t.Fatal("windows-11")
	}
	if !IsWindowsOS("Windows Server 2022") {
		t.Fatal("display name")
	}
	if IsWindowsOS("ubuntu-22.04") {
		t.Fatal("ubuntu should not be windows")
	}
}
