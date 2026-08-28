package hypervisor

import "strings"

// IsHypervisorJobFailed reports immediate hypervisor async job failure.
func IsHypervisorJobFailed(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed") &&
		(strings.Contains(msg, "openstack:") ||
			strings.Contains(msg, "queue") ||
			strings.Contains(msg, "rebuild") ||
			strings.Contains(msg, "change admin password"))
}

// IsIPPoolExhausted reports no free IPv4 in the pool (transient — retry/waitlist).
func IsIPPoolExhausted(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not enough addresses") ||
		strings.Contains(msg, "no free ipv4") ||
		strings.Contains(msg, "no free ip") ||
		strings.Contains(msg, "floating ip")
}

// IsTransientTemplateError reports image/flavor faults that can clear after sync.
func IsTransientTemplateError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid template id") ||
		strings.Contains(msg, "being initialized") ||
		strings.Contains(msg, "image") && strings.Contains(msg, "not found")
}

// IsPermanentProvisionError reports config faults that will not heal on retry.
func IsPermanentProvisionError(err error) bool {
	if err == nil {
		return false
	}
	if IsIPPoolExhausted(err) || IsTransientTemplateError(err) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no flavor mapping") ||
		strings.Contains(msg, "no image mapping") ||
		strings.Contains(msg, "invalid hypervisor") ||
		strings.Contains(msg, "flavor") && strings.Contains(msg, "not found") ||
		strings.Contains(msg, `"422"`) ||
		strings.Contains(msg, "http 422")
}
