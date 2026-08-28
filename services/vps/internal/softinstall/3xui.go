package softinstall

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
)

//go:embed 3xui_bootstrap.sh
var threeXUIBootstrapScript string

func apply3xUI(ctx context.Context, ip, password string) (*Bundle, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, fmt.Errorf("missing server ip for 3x-ui bootstrap")
	}
	script := fmt.Sprintf("export SERVER_IP=%q\nexport VLESS_SNI=%q\n%s",
		ip, "www.5ka.ru", threeXUIBootstrapScript)

	installCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		installCtx, cancel = context.WithTimeout(ctx, 18*time.Minute)
		defer cancel()
	}

	out, err := sshavail.RunScriptOut(installCtx, ip, password, script)
	if err != nil {
		return nil, fmt.Errorf("3x-ui bootstrap: %w", err)
	}
	bundle, err := parseBundleOutput(out)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bundle.Profile) == "" {
		bundle.Profile = "3x-ui"
	}
	return bundle, nil
}
