package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsTransientSSHInstallErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("python3 install: sshavail: auth failed: ssh: handshake failed: read tcp 172.18.0.5:36408->89.125.42.5:22: read: connection reset by peer"), true},
		{fmt.Errorf("apt-get install failed"), false},
		{errors.New("ssh not ready for software install: connection refused"), true},
	}
	for _, tc := range cases {
		if got := isTransientSSHInstallErr(tc.err); got != tc.want {
			t.Fatalf("isTransientSSHInstallErr(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
