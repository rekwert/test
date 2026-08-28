package winfix

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
)

//go:embed Fix-WindowsShellApps.ps1
var shellAppsScript string

// ApplyShellAppsFix re-registers Search/Start UWP packages on a fresh Windows guest.
// Soft-fails when OpenSSH is unavailable (template without SSH).
func ApplyShellAppsFix(ctx context.Context, ip, password, osTemplateID string) error {
	if catalog.ResolveOSFamily(osTemplateID) != "windows" {
		return nil
	}
	ip = strings.TrimSpace(ip)
	password = strings.TrimSpace(password)
	if ip == "" || password == "" {
		return nil
	}

	user := catalog.ResolvePasswordResetUser(osTemplateID)
	script := "$env:VPS_FORCE_SHELL_FIX='1'\n" + shellAppsScript

	const attempts = 12
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if !sshavail.TCPOpen(ctx, ip, 22) {
			lastErr = fmt.Errorf("ssh port closed")
		} else if err := sshavail.CheckUserPassword(ctx, ip, user, password); err != nil {
			lastErr = err
		} else if err := sshavail.RunPowerShell(ctx, ip, user, password, script); err != nil {
			lastErr = err
		} else {
			log.Printf("windows shell fix %s: ok (attempt %d)", ip, attempt)
			return nil
		}
		if attempt < attempts {
			if err := sleepCtx(ctx, 25*time.Second); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("windows shell fix: %w", lastErr)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
