package openstack_test

import (
	"testing"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/openstack"
)

func TestLoadConfigPlanFlavorMap(t *testing.T) {
	t.Setenv("OPENSTACK_PLAN_MAP", "11111111-1111-1111-1111-111111111101:m1.small,11111111-1111-1111-1111-111111111102:m1.medium")
	t.Setenv("OPENSTACK_OS_MAP", "ubuntu-22.04:Ubuntu-22.04")

	cfg := openstack.LoadConfig()
	ref, ok := cfg.FlavorRef("11111111-1111-1111-1111-111111111101")
	if !ok || ref != "m1.small" {
		t.Fatalf("flavor ref = %q ok=%v", ref, ok)
	}
	img, ok := cfg.ImageRef("ubuntu-22.04")
	if !ok || img != "Ubuntu-22.04" {
		t.Fatalf("image ref = %q ok=%v", img, ok)
	}
	if !openstack.RegionEnabled("nl") {
		t.Fatal("expected all regions enabled when OPENSTACK_PROVISION_REGIONS unset")
	}

	t.Setenv("OPENSTACK_PROVISION_REGIONS", "nl,fi")
	cfg = openstack.LoadConfig()
	if !openstack.RegionEnabled("nl") || !openstack.RegionEnabled("fi") {
		t.Fatal("expected nl and fi enabled")
	}
	if openstack.RegionEnabled("de") {
		t.Fatal("expected de disabled")
	}
}

func TestConfigComputeHost(t *testing.T) {
	cfg := openstack.Config{
		NodeHosts: map[string]string{
			"11111111-1111-1111-1111-111111111001": "compute-nl-01",
		},
		HypervisorHosts: map[string]string{
			"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee": "compute-nl-02",
		},
	}
	if got := cfg.ComputeHost("11111111-1111-1111-1111-111111111001", ""); got != "compute-nl-01" {
		t.Fatalf("node map host = %q", got)
	}
	if got := cfg.ComputeHost("", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"); got != "compute-nl-02" {
		t.Fatalf("hv map host = %q", got)
	}
	if got := cfg.ComputeHost("", "compute-nl-03.example"); got != "compute-nl-03.example" {
		t.Fatalf("direct hostname = %q", got)
	}
}
