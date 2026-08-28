package sshpubkey

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestValid(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"ssh-rsa Sanya174", false},
		{"ssh-ed25519 AAAA", false},
		{"not-a-key", false},
		{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7", false},
	}
	for _, tc := range cases {
		if got := Valid(tc.in); got != tc.want {
			t.Fatalf("Valid(%q)=%v want %v", tc.in, got, tc.want)
		}
	}

	pub := mustTestAuthorizedKey(t)
	if !Valid(pub) {
		t.Fatalf("expected generated key to be valid: %s", pub)
	}
	if !Valid(pub + " user@host") {
		t.Fatal("expected key with comment to be valid")
	}
	valid, skipped := FilterValid([]string{"ssh-rsa Sanya174", pub, pub, "  "})
	if skipped != 1 || len(valid) != 1 {
		t.Fatalf("FilterValid: valid=%d skipped=%d want valid=1 skipped=1", len(valid), skipped)
	}
}

func mustTestAuthorizedKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(sshPub.Marshal())
}
