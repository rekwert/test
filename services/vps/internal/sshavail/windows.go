package sshavail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf16"
)

// RunPowerShell dials user@ip and runs a PowerShell script (Windows OpenSSH guests).
func RunPowerShell(ctx context.Context, ip, user, password, script string) error {
	script = strings.TrimSpace(script)
	if script == "" {
		return fmt.Errorf("sshavail: empty powershell script")
	}
	client, err := dialUserWithHostKeys(ctx, ip, user, password, nil)
	if err != nil {
		return err
	}
	defer client.Close()

	encoded, err := encodePowerShellCommand(script)
	if err != nil {
		return err
	}
	cmd := "powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " + encoded
	return run(client, cmd)
}

func encodePowerShellCommand(script string) (string, error) {
	runes := []rune(script)
	utf16Codes := utf16.Encode(runes)
	buf := make([]byte, len(utf16Codes)*2)
	for i, code := range utf16Codes {
		buf[i*2] = byte(code)
		buf[i*2+1] = byte(code >> 8)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
