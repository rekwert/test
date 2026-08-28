package handler

import "testing"

func TestGuestReachCandidatesDedupesAndOrders(t *testing.T) {
	got := guestReachCandidates("1.1.1.1", []string{"1.1.1.2", "1.1.1.1"}, []string{"1.1.1.3", "1.1.1.2"})
	want := []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
