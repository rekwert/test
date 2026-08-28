package store

import "testing"

func TestHasAPIKeyScope(t *testing.T) {
	tests := []struct {
		scopes []string
		need   string
		want   bool
	}{
		{[]string{"billing"}, "billing", true},
		{[]string{"billing.read"}, "billing", true},
		{[]string{"billing.topup"}, "billing", true},
		{[]string{"billing.read", "billing.topup"}, "billing", true},
		{[]string{"vps.read"}, "billing", false},
		{[]string{"billing.read", "vps.read"}, "billing.read", true},
	}
	for _, tc := range tests {
		if got := HasAPIKeyScope(tc.scopes, tc.need); got != tc.want {
			t.Fatalf("HasAPIKeyScope(%#v, %q) = %v, want %v", tc.scopes, tc.need, got, tc.want)
		}
	}
}
