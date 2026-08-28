package store

import "testing"

func TestApplyReachability(t *testing.T) {
	patch := NodeSyncPatch{
		Status:    "online",
		VFIP:      "95.216.1.155",
		VFEnabled: true,
	}
	ApplyReachability(&patch, false)
	if patch.Status != "offline" {
		t.Fatalf("status = %q, want offline", patch.Status)
	}
	if !patch.VFEnabled {
		t.Fatal("vf_enabled should stay true when only reachability fails")
	}

	online := NodeSyncPatch{Status: "online", VFIP: "95.216.1.155"}
	ApplyReachability(&online, true)
	if online.Status != "online" {
		t.Fatalf("status = %q, want online", online.Status)
	}

	noIP := NodeSyncPatch{Status: "online", VFIP: ""}
	ApplyReachability(&noIP, false)
	if noIP.Status != "online" {
		t.Fatalf("status = %q, want online when IP empty", noIP.Status)
	}
}
