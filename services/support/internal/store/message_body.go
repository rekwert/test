package store

import "strings"

// NormalizeMessageBody keeps internal line breaks and spaces; only normalizes
// line endings and trims trailing whitespace on the whole message.
func NormalizeMessageBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.TrimRight(body, " \t\n\r\u00a0")
}

// IsEmptyMessageBody reports whether a message has no visible text.
func IsEmptyMessageBody(body string) bool {
	return strings.TrimSpace(NormalizeMessageBody(body)) == ""
}
