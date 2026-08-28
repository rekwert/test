package ua

import "strings"

type Info struct {
	Browser    string
	OS         string
	DeviceType string
}

func Parse(userAgent string) Info {
	ua := strings.TrimSpace(userAgent)
	info := Info{Browser: "Unknown", OS: "Unknown", DeviceType: "desktop"}
	if ua == "" {
		return info
	}
	lower := strings.ToLower(ua)

	switch {
	case strings.Contains(lower, "iphone"), strings.Contains(lower, "ipad"), strings.Contains(lower, "ipod"):
		info.OS = "iOS"
		info.DeviceType = "mobile"
	case strings.Contains(lower, "android"):
		info.OS = "Android"
		info.DeviceType = "mobile"
	case strings.Contains(lower, "windows"):
		info.OS = "Windows"
	case strings.Contains(lower, "mac os x"), strings.Contains(lower, "macintosh"):
		info.OS = "macOS"
	case strings.Contains(lower, "linux"):
		info.OS = "Linux"
	}

	switch {
	case strings.Contains(lower, "edg/"):
		info.Browser = "Edge"
	case strings.Contains(lower, "opr/"), strings.Contains(lower, "opera"):
		info.Browser = "Opera"
	case strings.Contains(lower, "firefox/"):
		info.Browser = "Firefox"
	case strings.Contains(lower, "chrome/"), strings.Contains(lower, "crios/"):
		info.Browser = "Chrome"
	case strings.Contains(lower, "safari/"):
		info.Browser = "Safari"
	}

	if strings.Contains(lower, "mobile") && info.DeviceType == "desktop" {
		info.DeviceType = "mobile"
	}

	return info
}
