package hypervisor

import (
	"errors"
	"testing"
)

func TestIsRecoverablePasswordQueueError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("openstack: rebuild failed: Failed setting password"), true},
		{errors.New("openstack: change admin password failed: guest agent is not connected"), true},
		{errors.New("openstack: queue timeout waiting for finish"), false},
	}
	for _, tc := range cases {
		if got := IsRecoverablePasswordQueueError(tc.err); got != tc.want {
			t.Fatalf("IsRecoverablePasswordQueueError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
	if IsRecoverablePasswordQueueError(errors.New("openstack: rebuild failed: unknown")) {
		// common when guest agent is down but password was returned
	} else {
		t.Fatal("expected recoverable for unknown rebuild failure")
	}
	if IsRecoverablePasswordQueueError(errors.New("openstack: queue timeout waiting for finish")) {
		t.Fatal("timeout should not be recoverable")
	}
}
