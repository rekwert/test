package hypervisor

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsHypervisorJobFailed(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("timeout"), false},
		{errors.New("openstack: queue failed"), true},
		{errors.New("openstack: rebuild failed"), true},
		{errors.New("openstack: queue timeout waiting for finish"), false},
	}
	for _, tc := range cases {
		if got := IsHypervisorJobFailed(tc.err); got != tc.want {
			t.Fatalf("IsHypervisorJobFailed(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsIPPoolExhausted(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("openstack: not enough addresses available"), true},
		{errors.New("no free ipv4 available or assign failed"), true},
		{errors.New("invalid package"), false},
	}
	for _, tc := range cases {
		if got := IsIPPoolExhausted(tc.err); got != tc.want {
			t.Fatalf("IsIPPoolExhausted(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsTransientTemplateError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New(`openstack: invalid template id`), true},
		{errors.New(`openstack: image is being initialized`), true},
		{errors.New(`openstack: image not found`), true},
		{errors.New(`openstack: invalid package`), false},
	}
	for _, tc := range cases {
		if got := IsTransientTemplateError(tc.err); got != tc.want {
			t.Fatalf("IsTransientTemplateError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsPermanentProvisionError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("openstack POST /servers: timeout"), false},
		{fmt.Errorf(`openstack: no flavor mapping for plan "x"`), true},
		{errors.New(`openstack POST /servers: HTTP 422`), true},
		{errors.New("openstack: not enough addresses available"), false},
		{errors.New("no free ipv4 addresses"), false},
		{errors.New(`openstack: invalid template id`), false},
		{errors.New(`openstack: image is being initialized`), false},
	}
	for _, tc := range cases {
		if got := IsPermanentProvisionError(tc.err); got != tc.want {
			t.Fatalf("IsPermanentProvisionError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
