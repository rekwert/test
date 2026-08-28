package fieldcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const prefix = "enc:v1:"

// Cipher encrypts sensitive fields at rest using AES-256-GCM.
// When disabled (no key), values pass through unchanged for dev/test.
type Cipher struct {
	aead cipher.AEAD
}

func NewFromEnv(envKey string) (*Cipher, error) {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return &Cipher{}, nil
	}
	key, err := decodeKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", envKey, err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func decodeKey(raw string) ([]byte, error) {
	if dec, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(dec) == 32 {
		return dec, nil
	}
	if dec, err := base64.StdEncoding.DecodeString(raw); err == nil && len(dec) == 32 {
		return dec, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, errors.New("key must be 32 bytes (raw) or base64-encoded 32 bytes")
}

func (c *Cipher) Enabled() bool {
	return c != nil && c.aead != nil
}

func (c *Cipher) IsEncrypted(value string) bool {
	return strings.HasPrefix(value, prefix)
}

func (c *Cipher) Encrypt(plain string) (string, error) {
	if plain == "" || !c.Enabled() || c.IsEncrypted(plain) {
		return plain, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, sealed...)
	return prefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

func (c *Cipher) Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !c.IsEncrypted(stored) {
		return stored, nil
	}
	if !c.Enabled() {
		return "", errors.New("encrypted value but VPS_FIELD_ENCRYPTION_KEY is not configured")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", err
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	plain, err := c.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
