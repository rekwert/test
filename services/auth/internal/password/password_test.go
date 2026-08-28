package password

import "testing"

func TestValidate(t *testing.T) {
	if err := Validate("short"); err == nil {
		t.Fatal("expected error for short password")
	}
	if err := Validate("longenough1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
