package opsssh

import (
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

const envPrivateKey = "VPS_OPS_SSH_PRIVATE_KEY"

// Signer returns the platform automation SSH signer when VPS_OPS_SSH_PRIVATE_KEY is set.
func Signer() (ssh.Signer, bool) {
	raw := strings.TrimSpace(os.Getenv(envPrivateKey))
	if raw == "" {
		return nil, false
	}
	raw = strings.ReplaceAll(raw, `\n`, "\n")
	raw = strings.Trim(strings.TrimSpace(raw), "'\"")
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		// Allow raw OpenSSH one-liner private key without PEM headers.
		signer, err := ssh.ParsePrivateKey([]byte(raw))
		if err != nil {
			return nil, false
		}
		return signer, true
	}
	signer, err := ssh.ParsePrivateKey(pem.EncodeToMemory(block))
	if err != nil {
		return nil, false
	}
	return signer, true
}

// AuthorizedKeyLine returns the OpenSSH authorized_keys line for the ops key, or "".
func AuthorizedKeyLine() string {
	signer, ok := Signer()
	if !ok {
		return ""
	}
	pub := ssh.MarshalAuthorizedKey(signer.PublicKey())
	return strings.TrimSpace(string(pub))
}

// GenerateEd25519PEM creates a new ed25519 private key PEM for initial env setup.
func GenerateEd25519PEM() (string, string, error) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, "cloud-hustle-ops")
	if err != nil {
		return "", "", err
	}
	pemBytes := pem.EncodeToMemory(block)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return "", "", fmt.Errorf("signer: %w", err)
	}
	pub := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	return string(pemBytes), pub, nil
}
