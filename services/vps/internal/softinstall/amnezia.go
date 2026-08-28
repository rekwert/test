package softinstall

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
)

//go:embed amnezia_bootstrap.sh
var amneziaBootstrapScript string

func applyAmnezia(ctx context.Context, ip, password string) (*Bundle, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, fmt.Errorf("missing server ip for amnezia bootstrap")
	}
	script := fmt.Sprintf("export SERVER_IP=%q\nexport AWG_PORT=%q\n%s",
		ip, "443", amneziaBootstrapScript)

	installCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		installCtx, cancel = context.WithTimeout(ctx, 20*time.Minute)
		defer cancel()
	}

	out, err := sshavail.RunScriptOut(installCtx, ip, password, script)
	if err != nil {
		return nil, fmt.Errorf("amnezia bootstrap: %w", err)
	}
	bundle, err := parseBundleOutput(out)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bundle.Profile) == "" {
		bundle.Profile = "amnezia"
	}
	return bundle, nil
}
