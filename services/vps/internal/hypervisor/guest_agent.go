package hypervisor

import "strings"

// IsGuestAgentUnavailable reports hypervisor could not reach QEMU guest agent inside the VM.
func IsGuestAgentUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "guest agent is not connected") ||
		strings.Contains(msg, "guest agent is not responding") ||
		strings.Contains(msg, "qemu guest agent is not connected") ||
		strings.Contains(msg, "failed setting password")
}

// IsRecoverablePasswordQueueError allows SSH verification with expectedPassword when hypervisor
// password reset failed but the panel already returned the password.
func IsRecoverablePasswordQueueError(err error) bool {
	if !IsHypervisorJobFailed(err) {
		return false
	}
	if IsGuestAgentUnavailable(err) {
		return true
	}
	// Hypervisor often returns job failed with no exception message ("unknown").
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, " failed: unknown") || strings.HasSuffix(msg, " failed")
}

// IsNotCommissionedError reports hypervisor rejected an action because the VM is not ready.
func IsNotCommissionedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not commissioned")
}
