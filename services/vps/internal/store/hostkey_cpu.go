package store

import (
	"regexp"
	"strings"
)

var hostkeyGenericCoreRe = regexp.MustCompile(`(?i)^\d+\s*cores?\b`)

func hostkeyIsGenericCoreLabel(s string) bool {
	return hostkeyGenericCoreRe.MatchString(strings.TrimSpace(s))
}

// hostkeyCPUFromBMLine extracts a CPU label from Hostkey BM catalog lines, e.g.
// "BM E3-12xx/32/2x960GB SSD" -> "Intel Xeon E3-12xx".
func hostkeyCPUFromBMLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "[]\"'")
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "BM ") {
		s = strings.TrimSpace(s[3:])
	} else if !strings.Contains(upper, "RYZEN") &&
		!strings.Contains(upper, "EPYC") &&
		!strings.Contains(upper, "XEON") &&
		!strings.Contains(upper, "I9-") &&
		!strings.Contains(upper, "I7-") &&
		!strings.Contains(upper, "E5-") &&
		!strings.Contains(upper, "E3-") {
		return ""
	}
	if i := strings.Index(s, "/"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	if plus := strings.Index(s, " + "); plus > 0 {
		s = strings.TrimSpace(s[:plus])
	}
	return hostkeyFormatCPUFragment(s)
}

func hostkeyFormatCPUFragment(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)

	count := 1
	if strings.HasPrefix(lower, "2x") {
		count = 2
		raw = strings.TrimSpace(raw[2:])
		lower = strings.ToLower(raw)
	}

	switch {
	case strings.Contains(lower, "ryzen"):
		return formatHostkeyVendorCPU("AMD", raw, count)
	case strings.Contains(lower, "epyc"):
		return formatHostkeyVendorCPU("AMD", raw, count)
	case strings.HasPrefix(lower, "i9-") || strings.HasPrefix(lower, "i7-") || strings.HasPrefix(lower, "i5-"):
		return formatHostkeyVendorCPU("Intel Core", raw, count)
	case strings.HasPrefix(lower, "e5-") || strings.HasPrefix(lower, "e3-") || strings.Contains(lower, "xeon"):
		return formatHostkeyVendorCPU("Intel Xeon", raw, count)
	default:
		if count > 1 {
			return strings.TrimSpace(strings.Replace(raw, "x", "× ", 1))
		}
		return raw
	}
}

func formatHostkeyVendorCPU(vendor, model string, count int) string {
	model = strings.TrimSpace(model)
	if count <= 1 {
		return vendor + " " + model
	}
	return "2× " + vendor + " " + model
}
