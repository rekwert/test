package sshavail

import "testing"

func TestEncodePowerShellCommand(t *testing.T) {
	encoded, err := encodePowerShellCommand("Write-Host ok")
	if err != nil {
		t.Fatal(err)
	}
	if encoded == "" {
		t.Fatal("empty encoded command")
	}
}
