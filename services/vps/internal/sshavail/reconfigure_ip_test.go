package sshavail

import (
	"strings"
	"testing"
)

func TestBuildStaticIPScriptPersistsBeforeAsyncActivation(t *testing.T) {
	script := buildStaticIPScript("212.102.227.39", "212.102.227.1", 24)

	for _, required := range []string{
		`/etc/NetworkManager/system-connections/`,
		`/etc/sysconfig/network-scripts/ifcfg-`,
		`address $NEW/$PFX`,
		`nohup sh -c`,
		`ip addr replace "$NEW/$PFX"`,
		`CHANGE_SCHEDULED`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("script missing %q", required)
		}
	}
	if strings.Contains(script, "netplan apply") {
		t.Fatal("script must not apply netplan inside the active SSH session")
	}

	persistAt := strings.Index(script, "NM_FILE=")
	activateAt := strings.Index(script, "nohup sh -c")
	if persistAt < 0 || activateAt < 0 || persistAt >= activateAt {
		t.Fatalf("network persistence must precede activation: persist=%d activate=%d", persistAt, activateAt)
	}
}
