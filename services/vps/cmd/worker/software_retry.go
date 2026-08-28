package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

const claudeCodeTerminalPort = 7681
const claudeCodeTerminalThemeMarker = "v3-clipboard"

// runSoftwareRetryLoop isolates slow SSH/software work from provisioning,
// outbox and synchronization ticks. A fresh delay starts after each batch so
// a long install cannot create overlapping retries or an immediate retry storm.
func runSoftwareRetryLoop(ctx context.Context, st *store.Store, hv hypervisor.Adapter, mockMode bool, interval time.Duration) {
	if interval <= 0 {
		interval = 3 * time.Minute
	}
	for {
		if err := processSoftwareInstallRetries(ctx, st, hv, mockMode); err != nil {
			log.Printf("vps worker software retry: %v", err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func claudeCodeTerminalHealthy(ctx context.Context, ip string) bool {
	if !sshavail.TCPOpen(ctx, ip, claudeCodeTerminalPort) {
		return false
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		fmt.Sprintf("http://%s:%d/claude-terminal.js", ip, claudeCodeTerminalPort),
		nil,
	)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	return strings.Contains(body, claudeCodeTerminalThemeMarker)
}

func processSoftwareInstallRetries(ctx context.Context, st *store.Store, hv hypervisor.Adapter, mockMode bool) error {
	if mockMode {
		return nil
	}
	failed, err := st.ListSoftwareInstallRetries(ctx, 3)
	if err != nil {
		return err
	}
	for _, item := range failed {
		if err := retrySoftwareInstall(ctx, st, hv, item); err != nil {
			log.Printf("software retry %s (%s): %v", item.ID, item.SoftwareProfileID, err)
			continue
		}
		log.Printf("software retry %s (%s): ok", item.ID, item.SoftwareProfileID)
	}

	health, err := st.ListClaudeCodeHealthChecks(ctx, 5)
	if err != nil {
		return err
	}
	for _, item := range health {
		item.IP = strings.TrimSpace(item.IP)
		if item.IP == "" {
			continue
		}
		if claudeCodeTerminalHealthy(ctx, item.IP) {
			continue
		}
		if err := retrySoftwareInstall(ctx, st, hv, item); err != nil {
			log.Printf("software retry %s (%s): %v", item.ID, item.SoftwareProfileID, err)
			continue
		}
		log.Printf("software retry %s (%s): ok", item.ID, item.SoftwareProfileID)
	}
	return nil
}

func retrySoftwareInstall(ctx context.Context, st *store.Store, hv hypervisor.Adapter, item store.SoftwareInstallRetry) error {
	if err := st.MarkSoftwareInstallRetry(ctx, item.ID); err != nil {
		return err
	}
	password, err := st.GetInstanceRootPassword(ctx, item.ID)
	if err != nil {
		return err
	}
	if err := sshavail.CheckRootPassword(ctx, item.IP, password); err != nil {
		password, err = syncRootPassword(ctx, st, hv, item.ID, item.ExternalID, item.IP, password, item.OSTemplateID)
		if err != nil || strings.TrimSpace(password) == "" {
			return err
		}
	}
	ci := store.CreatingInstance{
		ID:                item.ID,
		OSTemplateID:      item.OSTemplateID,
		SoftwareProfileID: item.SoftwareProfileID,
	}
	return applySoftwareProfile(ctx, st, ci, item.IP, password)
}
