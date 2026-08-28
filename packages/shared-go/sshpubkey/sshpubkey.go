package sshpubkey

import (
	"strings"

	"golang.org/x/crypto/ssh"
)

// Normalize returns "type base64" without comment, or empty if unusable.
func Normalize(raw string) string {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	return strings.Join(parts, " ")
}

// Valid reports whether raw is a parseable OpenSSH authorized_keys line
// (ssh-rsa / ssh-ed25519 / ecdsa-*, etc.). Rejects placeholders like "ssh-rsa Sanya174".
func Valid(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 8192 {
		return false
	}
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
	return err == nil
}

// FilterValid keeps only parseable keys, preserving original lines (with comments).
func FilterValid(keys []string) (valid []string, skipped int) {
	seen := map[string]struct{}{}
	for _, raw := range keys {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !Valid(raw) {
			skipped++
			continue
		}
		norm := Normalize(raw)
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		valid = append(valid, raw)
	}
	return valid, skipped
}
