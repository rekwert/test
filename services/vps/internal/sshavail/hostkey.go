package sshavail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
)

// HostKeyCallback verifies host keys against trustedPubKeys when provided;
// falls back to InsecureIgnoreHostKey only when no keys are configured.
func HostKeyCallback(trustedPubKeys []string) ssh.HostKeyCallback {
	trusted := parseTrustedHostKeys(trustedPubKeys)
	if len(trusted) == 0 {
		return ssh.InsecureIgnoreHostKey() //nolint:gosec
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		for _, want := range trusted {
			if bytes.Equal(want.Marshal(), key.Marshal()) {
				return nil
			}
		}
		return fmt.Errorf("ssh host key mismatch")
	}
}

func parseTrustedHostKeys(keys []string) []ssh.PublicKey {
	var out []ssh.PublicKey
	for _, raw := range keys {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
		if err == nil {
			out = append(out, pub)
			continue
		}
		if dec, err := base64.StdEncoding.DecodeString(raw); err == nil {
			if pub, err := ssh.ParsePublicKey(dec); err == nil {
				out = append(out, pub)
			}
		}
	}
	return out
}
