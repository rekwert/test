package store

import "testing"

func TestNormalizeMessageBody(t *testing.T) {
	got := NormalizeMessageBody("  line1\r\nline2\r\n  ")
	if got != "  line1\nline2" {
		t.Fatalf("got %q", got)
	}
	if !IsEmptyMessageBody("   ") {
		t.Fatal("expected spaces-only body to be empty")
	}
	if IsEmptyMessageBody("hello") {
		t.Fatal("expected hello to be non-empty")
	}
}
