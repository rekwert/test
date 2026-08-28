package openstack

import "testing"

func TestCIDRPrefix(t *testing.T) {
	if got := cidrPrefix("192.168.0.0/24"); got != 24 {
		t.Fatalf("prefix = %d, want 24", got)
	}
	if got := cidrPrefix("bad"); got != 0 {
		t.Fatalf("prefix = %d, want 0", got)
	}
}

func TestPrefixToNetmask(t *testing.T) {
	if got := prefixToNetmask(24); got != "255.255.255.0" {
		t.Fatalf("netmask = %q", got)
	}
}

func TestDedupeStrings(t *testing.T) {
	got := dedupeStrings([]string{"1.1.1.1", "1.1.1.1", "", "2.2.2.2"})
	if len(got) != 2 || got[0] != "1.1.1.1" || got[1] != "2.2.2.2" {
		t.Fatalf("got %v", got)
	}
}

func TestIsFloatingPoolExhausted(t *testing.T) {
	if !isFloatingPoolExhausted(fmtError("503 not enough addresses")) {
		t.Fatal("expected pool exhausted")
	}
	if isFloatingPoolExhausted(fmtError("timeout")) {
		t.Fatal("expected not exhausted")
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
