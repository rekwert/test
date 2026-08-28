package sshavail

import (
	"strings"
	"testing"
)

func TestShellSingleQuote(t *testing.T) {
	if got := shellSingleQuote("abc"); got != "'abc'" {
		t.Fatalf("got %q", got)
	}
	if got := shellSingleQuote("a'b"); got != "'a'\\''b'" {
		t.Fatalf("got %q", got)
	}
}

func TestWindowsChangePasswordCommand(t *testing.T) {
	cmd := windowsChangePasswordCommand("Administrator", "P@ss w0rd!")
	if !strings.Contains(cmd, "FromBase64String") {
		t.Fatalf("expected base64 decode in command: %q", cmd)
	}
	if !strings.Contains(cmd, "Administrator") {
		t.Fatalf("expected username in command: %q", cmd)
	}
}
