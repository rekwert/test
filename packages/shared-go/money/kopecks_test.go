package money

import "testing"

func TestFromRubles(t *testing.T) {
	if got := FromRubles(10.5); got != 1050 {
		t.Fatalf("FromRubles(10.5) = %d, want 1050", got)
	}
	if got := FromRubles(0.01); got != 1 {
		t.Fatalf("FromRubles(0.01) = %d, want 1", got)
	}
}

func TestToRubles(t *testing.T) {
	if got := Kopecks(199).ToRubles(); got != 1.99 {
		t.Fatalf("ToRubles = %v, want 1.99", got)
	}
}
