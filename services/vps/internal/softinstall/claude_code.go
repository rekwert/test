package softinstall

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
)

//go:embed claude_code_bootstrap.sh
var claudeCodeBootstrapScript string

type ClaudeAccess struct {
	LoginHint    string `json:"login_hint,omitempty"`
	StartCommand string `json:"start_command,omitempty"`
}

func applyClaudeCode(ctx context.Context, ip, password string) (*Bundle, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, fmt.Errorf("missing server ip for claude-code bootstrap")
	}
	script := fmt.Sprintf("export SERVER_IP=%q\n%s", ip, claudeCodeBootstrapScript)

	installCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		installCtx, cancel = context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
	}

	out, err := sshavail.RunScriptOut(installCtx, ip, password, script)
	if err != nil {
		return nil, fmt.Errorf("claude-code bootstrap: %w", err)
	}
	bundle, err := parseBundleOutput(out)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bundle.Profile) == "" {
		bundle.Profile = "claude-code"
	}
	return bundle, nil
}
