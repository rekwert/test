package store

import "testing"

func TestNormalizeAPIKeyScopes(t *testing.T) {
	got, err := NormalizeAPIKeyScopes(nil)
	if err != nil || len(got) != 1 || got[0] != "billing" {
		t.Fatalf("default scopes: got %#v err=%v", got, err)
	}
	got, err = NormalizeAPIKeyScopes([]string{"billing.topup", "billing.read", "billing.read"})
	if err != nil || len(got) != 1 || got[0] != "billing" {
		t.Fatalf("collapse customer billing scopes: got %#v err=%v", got, err)
	}
	got, err = NormalizeAPIKeyScopes([]string{"billing.read", "vps.read"})
	if err != nil || len(got) != 2 || got[0] != "billing.read" || got[1] != "vps.read" {
		t.Fatalf("reseller scopes unchanged: got %#v err=%v", got, err)
	}
	if _, err := NormalizeAPIKeyScopes([]string{"admin"}); err == nil {
		t.Fatal("expected unsupported scope error")
	}
}

func TestNewAPIKeySecret(t *testing.T) {
	raw, prefix, hash, err := NewAPIKeySecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 20 || prefix != raw[:16] || hash != HashAPIKey(raw) {
		t.Fatalf("bad secret raw=%s prefix=%s hash=%s", raw, prefix, hash)
	}
}
