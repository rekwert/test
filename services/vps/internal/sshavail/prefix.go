package sshavail

import (
	"net"
	"strings"
)

// PrefixFromNetmask converts an IPv4 netmask string to a CIDR prefix length.
func PrefixFromNetmask(netmask string) int {
	netmask = strings.TrimSpace(netmask)
	if netmask == "" {
		return 24
	}
	ip := net.ParseIP(netmask)
	if ip == nil {
		return 24
	}
	ones, bits := net.IPMask(ip.To4()).Size()
	if bits == 32 && ones > 0 && ones <= 32 {
		return ones
	}
	return 24
}
