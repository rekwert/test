package catalog

import "strings"

// WindowsLoginUser is the VirtFusion guest-agent account for Windows templates.
// VF validates resetPassword user names against the image metadata; Win10/Server
// images expose "Administrator", not "root" or custom cloud-init users.
func WindowsLoginUser() string {
	return "Administrator"
}

// ResolvePasswordResetUser returns the hypervisor resetPassword user for an OS template.
func ResolvePasswordResetUser(osTemplateID string) string {
	if ResolveOSFamily(osTemplateID) == "windows" {
		return WindowsLoginUser()
	}
	return "root"
}

// IsWindowsOS reports whether the template id or display name is a Windows image.
func IsWindowsOS(osTemplateOrName string) bool {
	return ResolveOSFamily(osTemplateOrName) == "windows" ||
		strings.Contains(strings.ToLower(osTemplateOrName), "windows")
}
