package hypervisor

import (
	"os"
	"testing"
)

func TestActiveOSTemplateMapFromEnv(t *testing.T) {
	t.Setenv("OPENSTACK_OS_MAP", "ubuntu-22.04:img-1,debian-12:img-2")
	m := ActiveOSTemplateMap()
	if len(m) != 2 {
		t.Fatalf("got %d entries, want 2", len(m))
	}
	if _, ok := m["ubuntu-22.04"]; !ok {
		t.Fatal("missing ubuntu-22.04")
	}
	if _, ok := m["debian-12"]; !ok {
		t.Fatal("missing debian-12")
	}
}

func TestHasExplicitPlan(t *testing.T) {
	t.Setenv("OPENSTACK_PLAN_MAP", "11111111-1111-1111-1111-111111111101:m1.small")
	if !HasExplicitPlan("11111111-1111-1111-1111-111111111101") {
		t.Fatal("expected explicit plan")
	}
	if HasExplicitPlan("missing-plan") {
		t.Fatal("expected missing plan")
	}
	_ = os.Getenv("OPENSTACK_PLAN_MAP")
}
