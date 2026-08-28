package fieldcrypto

import (
	"os"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv("VPS_FIELD_ENCRYPTION_KEY", "01234567890123456789012345678901")
	c, err := NewFromEnv("VPS_FIELD_ENCRYPTION_KEY")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := c.Encrypt("s3cret-root-pass")
	if err != nil {
		t.Fatal(err)
	}
	if enc == "s3cret-root-pass" || !c.IsEncrypted(enc) {
		t.Fatalf("expected encrypted payload, got %q", enc)
	}
	plain, err := c.Decrypt(enc)
	if err != nil || plain != "s3cret-root-pass" {
		t.Fatalf("decrypt = %q err=%v", plain, err)
	}
}

func TestPassthroughWhenDisabled(t *testing.T) {
	os.Unsetenv("VPS_FIELD_ENCRYPTION_KEY")
	c, err := NewFromEnv("VPS_FIELD_ENCRYPTION_KEY")
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Encrypt("plain")
	if err != nil || out != "plain" {
		t.Fatalf("encrypt passthrough = %q err=%v", out, err)
	}
	plain, err := c.Decrypt("legacy-plain")
	if err != nil || plain != "legacy-plain" {
		t.Fatalf("decrypt legacy = %q err=%v", plain, err)
	}
}
