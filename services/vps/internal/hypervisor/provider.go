package hypervisor

import (
	"os"
	"strings"
)

const DefaultVPSProvider = "openstack"

// Provider returns the active hypervisor backend: mock or openstack.
func Provider() string {
	if MockEnabled() {
		return "mock"
	}
	if OpenStackConfigured() {
		return "openstack"
	}
	return "mock"
}

// OpenStackConfigured reports whether OpenStack credentials are present.
func OpenStackConfigured() bool {
	return strings.TrimSpace(os.Getenv("OPENSTACK_AUTH_URL")) != "" ||
		strings.TrimSpace(os.Getenv("OS_AUTH_URL")) != ""
}

// OpenStackEnabled is true when OpenStack (not mock) should be used.
func OpenStackEnabled() bool {
	return !MockEnabled() && OpenStackConfigured()
}
