package smtpblock

import (
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	got := shellQuote("a'b")
	if !strings.Contains(got, `'\''`) {
		t.Fatalf("quote escaped: %q", got)
	}
}

func TestPorts(t *testing.T) {
	if len(Ports) != 2 || Ports[0] != 25 || Ports[1] != 2525 {
		t.Fatalf("unexpected ports: %#v", Ports)
	}
}

func TestHVJumpHost(t *testing.T) {
	t.Setenv("HV_SSH_JUMP_HOST", "66.248.206.14")
	if got := hvJumpHost("212.102.227.7"); got != "66.248.206.14" {
		t.Fatalf("jump host: got %q", got)
	}
	if got := hvJumpHost("212.102.227.6"); got != "" {
		t.Fatalf("direct host should not jump: got %q", got)
	}
}
