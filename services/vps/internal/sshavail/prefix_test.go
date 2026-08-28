package sshavail

import "testing"

func TestPrefixFromNetmask(t *testing.T) {
	if got := PrefixFromNetmask("255.255.255.0"); got != 24 {
		t.Fatalf("got %d want 24", got)
	}
	if got := PrefixFromNetmask(""); got != 24 {
		t.Fatalf("empty got %d want 24", got)
	}
	if got := PrefixFromNetmask("255.255.255.255"); got != 32 {
		t.Fatalf("got %d want 32", got)
	}
}
