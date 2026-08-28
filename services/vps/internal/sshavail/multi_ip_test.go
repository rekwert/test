package sshavail

import (
	"strings"
	"testing"
)

func TestNormalizeIPListPrimaryFirst(t *testing.T) {
	got := normalizeIPList([]string{"91.108.247.6", "91.108.247.2", "91.108.247.4"}, "91.108.247.3")
	want := []string{"91.108.247.3", "91.108.247.2", "91.108.247.4", "91.108.247.6"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestBuildMultiIPScriptContainsAllAddresses(t *testing.T) {
	script := buildMultiIPScript(
		[]string{"91.108.247.3", "91.108.247.6"},
		"91.108.247.3",
		"91.108.247.1",
		24,
	)
	for _, ip := range []string{"91.108.247.3", "91.108.247.6", "91.108.247.1"} {
		if !strings.Contains(script, ip) {
			t.Fatalf("script missing %s", ip)
		}
	}
}
