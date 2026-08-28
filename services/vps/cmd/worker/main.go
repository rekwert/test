package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/sshpubkey"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/abuse"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hvinit"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/opsssh"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/softinstall"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/winfix"
)

func main() {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("POSTGRES_DSN is required for vps worker")
	}
	prodenv.AssertPostgresDSNSecurity(dsn)

	ctx := context.Background()
	st, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("vps worker store: %v", err)
	}
	defer st.Close()

	hv := hvinit.NewAdapter()
	hypervisor.AssertProductionHypervisor()
	nodeSync := hvinit.NewNodeSync()
	osSync := hvinit.NewOSSync()
	robotCfg := hetznerrobot.LoadConfig()
	robot := hetznerrobot.NewFromEnv()
	hostkeyCfg := hostkey.LoadConfig()
	hostkeyClient := hostkey.NewFromEnv()
	mockMode := hypervisor.MockEnabled()
	switch hypervisor.Provider() {
	case "mock":
		log.Printf("vps worker: hypervisor mock mode")
	case "openstack":
		log.Printf("vps worker: openstack mode (region=%s auth=%s)", os.Getenv("OPENSTACK_REGION"), os.Getenv("OPENSTACK_AUTH_URL"))
	default:
		log.Printf("vps worker: hypervisor mode (%s)", hypervisor.Provider())
	}
	if robotCfg.Enabled {
		rate, _, src := hetznerrobot.CachedEurRub()
		if rate <= 0 {
			rate = robotCfg.EurRub
			src = "cfg"
		}
		log.Printf("vps worker: hetzner robot enabled (markup=%.1f%% eur_rub=%.4f fx=%s)", robotCfg.MarkupPercent, rate, src)
	} else {
		log.Printf("vps worker: hetzner robot mock mode")
	}
	if hostkeyCfg.Enabled {
		log.Printf("vps worker: hostkey invapi enabled (markup=%.1f%%)", hostkeyCfg.MarkupPercent)
	} else {
		log.Printf("vps worker: hostkey mock mode")
	}

	interval := envDuration("VPS_WORKER_INTERVAL", 5*time.Second)
	log.Printf("vps worker: guest-agent warmup=%s vf-reset-gap=%s (all OS templates)",
		guestAgentWarmupDuration(), hvResetMinGap())
	nodeSyncInterval := envDuration("VPS_NODE_SYNC_INTERVAL", 2*time.Minute)
	osSyncInterval := envDuration("VPS_OS_SYNC_INTERVAL", 10*time.Minute)
	instanceSyncInterval := envDuration("VPS_INSTANCE_SYNC_INTERVAL", 3*time.Minute)
	dedicatedSyncInterval := robotCfg.SyncInterval
	lastNodeSync := time.Time{}
	lastOSSync := time.Time{}
	lastInstanceSync := time.Time{}
	lastDedicatedSync := time.Time{}
	lastHostkeySync := time.Time{}
	hostkeySyncInterval := hostkeyCfg.SyncInterval
	lastPriceReview := time.Time{}
	priceReviewInterval := envDuration("VPS_DEDICATED_PRICE_REVIEW_INTERVAL", 1*time.Hour)
	lastAbuseScan := time.Time{}
	abuseScanInterval := envDuration("ABUSE_SCAN_INTERVAL", 2*time.Minute)
	softwareRetryInterval := envDuration("VPS_SOFTWARE_RETRY_INTERVAL", 3*time.Minute)
	metricsInterval := envDuration("VPS_METRICS_INTERVAL", 45*time.Second)
	lastMetricsRefresh := time.Time{}
	abuseSvc := abuse.NewService(abuse.LoadConfig(), st, hv)
	if abuseSvc.Enabled() {
		log.Printf("vps worker: abuse detection enabled (scan=%s threshold=%d)", abuseScanInterval, abuse.LoadConfig().AutoStopThreshold)
	}

	loadOSMapFromDB(ctx, st)
	if err := syncOS(ctx, st, osSync); err != nil {
		log.Printf("vps worker initial os sync: %v", err)
	}
	if err := syncDedicatedCatalog(ctx, st, robot, robotCfg); err != nil {
		log.Printf("vps worker initial dedicated sync: %v", err)
	}
	if err := syncHostkeyCatalog(ctx, st, hostkeyClient, hostkeyCfg); err != nil {
		log.Printf("vps worker initial hostkey sync: %v", err)
	}
	lastDedicatedSync = time.Now()
	lastHostkeySync = time.Now()

	if err := st.ReencryptLegacySecrets(ctx); err != nil {
		log.Printf("vps worker legacy secret reencrypt: %v", err)
	}
	go runSoftwareRetryLoop(ctx, st, hv, mockMode, softwareRetryInterval)

	log.Printf("vps worker started (interval=%s, node_sync=%s, os_sync=%s, dedicated_sync=%s, metrics=%s)", interval, nodeSyncInterval, osSyncInterval, dedicatedSyncInterval, metricsInterval)
	pollBackoff := newPollBackoffTracker()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	run := func() {
		if err := processWaitlist(ctx, st, hv); err != nil {
			log.Printf("vps worker waitlist: %v", err)
		}
		if err := processOutbox(ctx, st, hv, robot, hostkeyClient); err != nil {
			log.Printf("vps worker outbox: %v", err)
		}
		if err := processCreating(ctx, st, hv, mockMode, pollBackoff); err != nil {
			log.Printf("vps worker creating: %v", err)
		}
		if err := processDedicatedCreating(ctx, st, robot); err != nil {
			log.Printf("vps worker dedicated creating: %v", err)
		}
		if err := processHostkeyDedicatedCreating(ctx, st, hostkeyClient); err != nil {
			log.Printf("vps worker hostkey dedicated creating: %v", err)
		}
		if err := processDedicatedExtraIPs(ctx, st, robot); err != nil {
			log.Printf("vps worker dedicated extra ips: %v", err)
		}
		if err := processReinstalling(ctx, st, hv); err != nil {
			log.Printf("vps worker reinstalling: %v", err)
		}
		if time.Since(lastMetricsRefresh) >= metricsInterval {
			if err := refreshMetrics(ctx, st, hv); err != nil {
				log.Printf("vps worker metrics: %v", err)
			}
			lastMetricsRefresh = time.Now()
		}
		if abuseSvc.Enabled() && time.Since(lastAbuseScan) >= abuseScanInterval {
			if err := abuseSvc.ScanBatch(ctx); err != nil {
				log.Printf("vps worker abuse scan: %v", err)
			}
			lastAbuseScan = time.Now()
		}
		if time.Since(lastNodeSync) >= nodeSyncInterval {
			if err := syncNodes(ctx, st, nodeSync); err != nil {
				log.Printf("vps worker node sync: %v", err)
			}
			lastNodeSync = time.Now()
		}
		if time.Since(lastOSSync) >= osSyncInterval {
			if err := syncOS(ctx, st, osSync); err != nil {
				log.Printf("vps worker os sync: %v", err)
			}
			lastOSSync = time.Now()
		}
		if time.Since(lastInstanceSync) >= instanceSyncInterval {
			if err := syncInstanceStates(ctx, st, hv); err != nil {
				log.Printf("vps worker instance sync: %v", err)
			}
			lastInstanceSync = time.Now()
		}
		if time.Since(lastDedicatedSync) >= dedicatedSyncInterval {
			if err := syncDedicatedCatalog(ctx, st, robot, robotCfg); err != nil {
				log.Printf("vps worker dedicated sync: %v", err)
			}
			lastDedicatedSync = time.Now()
		}
		if time.Since(lastHostkeySync) >= hostkeySyncInterval {
			if err := syncHostkeyCatalog(ctx, st, hostkeyClient, hostkeyCfg); err != nil {
				log.Printf("vps worker hostkey sync: %v", err)
			}
			lastHostkeySync = time.Now()
		}
		if time.Since(lastPriceReview) >= priceReviewInterval {
			if err := processDedicatedPriceNotices(ctx, st); err != nil {
				log.Printf("vps worker dedicated price review: %v", err)
			}
			lastPriceReview = time.Now()
		}
	}

	run()
	for range ticker.C {
		run()
	}
}

func processWaitlist(ctx context.Context, st *store.Store, hv hypervisor.Adapter) error {
	n, err := st.PromoteWaitlisted(ctx, 10, func(ctx context.Context, region, hypervisorID string) (bool, error) {
		return hv.HasFreePrimaryIPv4(ctx, region, hypervisorID)
	})
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("waitlist promoted %d instance(s) to creating", n)
	}
	return nil
}

func processOutbox(ctx context.Context, st *store.Store, hv hypervisor.Adapter, robot hetznerrobot.Client, hk hostkey.Client) error {
	events, err := st.FetchPendingOutbox(ctx, 20)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	workers := envInt("VPS_OUTBOX_WORKERS", 4)
	if workers <= 1 {
		for _, ev := range events {
			if err := handleOutboxEvent(ctx, st, hv, robot, hk, ev); err != nil {
				log.Printf("outbox event %d: %v", ev.ID, err)
			}
		}
		return nil
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, ev := range events {
		wg.Add(1)
		sem <- struct{}{}
		go func(ev store.OutboxEvent) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := handleOutboxEvent(ctx, st, hv, robot, hk, ev); err != nil {
				log.Printf("outbox event %d: %v", ev.ID, err)
			}
		}(ev)
	}
	wg.Wait()
	return nil
}

func handleOutboxEvent(ctx context.Context, st *store.Store, hv hypervisor.Adapter, robot hetznerrobot.Client, hk hostkey.Client, ev store.OutboxEvent) error {
	var handleErr error
	switch ev.EventType {
	case "instance.provision_requested":
		handleErr = handleProvision(ctx, st, hv, ev.Payload)
	case "instance.reinstall_requested":
		handleErr = handleReinstall(ctx, st, hv, ev.Payload)
	case "instance.destroy_requested":
		handleErr = handleDestroy(ctx, st, hv, ev.Payload)
	case "instance.start_requested":
		handleErr = handleStart(ctx, st, hv, ev.Payload)
	case "instance.stop_requested":
		handleErr = handleStop(ctx, st, hv, ev.Payload)
	case "dedicated.provision_requested":
		handleErr = routeDedicatedProvision(ctx, st, robot, hk, ev.Payload)
	case "instance.password_change_requested":
		handleErr = handlePasswordChange(ctx, st, hv, ev.Payload)
	default:
		log.Printf("outbox: unknown event_type %q id=%d", ev.EventType, ev.ID)
		handleErr = fmt.Errorf("unknown outbox event_type: %s", ev.EventType)
	}
	if handleErr != nil {
		log.Printf("%s event %d: %v", ev.EventType, ev.ID, handleErr)
		if ev.EventType == "instance.provision_requested" && hypervisor.IsPermanentProvisionError(handleErr) {
			instanceID := outboxInstanceID(ev.Payload)
			if instanceID != "" {
				host := outboxHostname(ev.Payload)
				finalizeFailedProvision(ctx, st, hv, instanceID, outboxUserID(ev.Payload), host, handleErr.Error(), notify.FailProvision)
			}
			_ = st.MarkOutboxPublished(ctx, ev.ID)
		} else {
			_ = st.ReleaseOutboxClaim(ctx, ev.ID)
		}
		return handleErr
	}
	// Provision/reinstall events only kick off work. Instance pollers own completion,
	// so keeping these rows unpublished would let old builds starve newer events.
	_ = st.MarkOutboxPublished(ctx, ev.ID)
	return nil
}

func handleProvision(ctx context.Context, st *store.Store, hv hypervisor.Adapter, payload json.RawMessage) error {
	var data struct {
		InstanceID        string   `json:"instance_id"`
		UserID            string   `json:"user_id"`
		PlanID            string   `json:"plan_id"`
		Region            string   `json:"region"`
		NodeID            string   `json:"node_id"`
		Hostname          string   `json:"hostname"`
		OSTemplateID      string   `json:"os_template_id"`
		SoftwareProfileID string   `json:"software_profile_id"`
		RootPassword      string   `json:"root_password"`
		SSHKeys           []string `json:"ssh_keys"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	if strings.TrimSpace(data.RootPassword) == "" {
		pwd, pwdErr := st.GetInstanceRootPassword(ctx, data.InstanceID)
		if pwdErr != nil {
			return pwdErr
		}
		data.RootPassword = pwd
	}

	state, err := st.GetInstanceState(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if state != "creating" {
		return nil
	}

	opts := provisionCreateOpts(ctx, st, data.NodeID, data.PlanID, data.Region, data.Hostname, data.OSTemplateID, data.RootPassword, buildSSHKeys(data.OSTemplateID, data.SSHKeys))

	externalID, err := st.GetInstanceExternalID(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if externalID == "" {
		var ok bool
		externalID, ok, err = ensureHypervisorServerAllocated(ctx, st, hv, data.InstanceID, opts)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}

	server, err := hv.GetServer(ctx, externalID)
	if err != nil {
		return fmt.Errorf("poll server before build: %w", err)
	}
	if hypervisor.ServerNeedsBuild(server) {
		if err := hv.BuildServer(ctx, externalID, opts); err != nil {
			log.Printf("provision build %s vf=%s: %v", data.InstanceID, externalID, err)
			if hypervisor.IsIPPoolExhausted(err) {
				_ = requeueForIPPool(ctx, st, hv, data.InstanceID, externalID)
				return nil
			}
			return err
		}
	}

	// Guest setup, password sync and software work can take minutes. Keep the
	// outbox fast and let processCreating finish it under an instance claim.
	return nil
}

func handleReinstall(ctx context.Context, st *store.Store, hv hypervisor.Adapter, payload json.RawMessage) error {
	var data struct {
		InstanceID   string   `json:"instance_id"`
		OSTemplateID string   `json:"os_template_id"`
		RootPassword string   `json:"root_password"`
		SSHKeys      []string `json:"ssh_keys"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	if strings.TrimSpace(data.RootPassword) == "" {
		pwd, pwdErr := st.GetInstanceRootPassword(ctx, data.InstanceID)
		if pwdErr != nil {
			return pwdErr
		}
		data.RootPassword = pwd
	}
	state, err := st.GetInstanceState(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if state != "reinstalling" {
		return nil
	}
	externalID, err := st.GetInstanceExternalID(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	planID, _ := st.GetInstancePlanID(ctx, data.InstanceID)
	sshKeys := buildSSHKeys(data.OSTemplateID, data.SSHKeys)
	started, err := st.TryMarkReinstallBuildStarted(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	if err := hv.ReinstallServer(ctx, externalID, planID, data.OSTemplateID, data.RootPassword, sshKeys); err != nil {
		_ = st.ClearReinstallBuildStarted(ctx, data.InstanceID)
		return err
	}
	clearGuestAgentWarmup(ctx, st, data.InstanceID)
	clearVFPasswordResetCache(ctx, st, data.InstanceID)
	return nil
}

func handleDestroy(ctx context.Context, st *store.Store, hv hypervisor.Adapter, payload json.RawMessage) error {
	var data struct {
		InstanceID string `json:"instance_id"`
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	externalID := strings.TrimSpace(data.ExternalID)
	if externalID == "" && data.InstanceID != "" {
		id, err := st.GetInstanceExternalID(ctx, data.InstanceID)
		if err != nil {
			return err
		}
		externalID = id
	}
	if externalID == "" {
		return nil
	}
	_ = hv.StopServer(ctx, externalID)
	if err := hv.DeleteServer(ctx, externalID); err != nil {
		if !hypervisor.IsServerNotFound(err) && !hypervisor.IsNotCommissionedError(err) {
			log.Printf("destroy vf=%s instance=%s: %v", externalID, data.InstanceID, err)
			return err
		}
		log.Printf("destroy vf=%s instance=%s: already absent or not commissioned", externalID, data.InstanceID)
	}
	if data.InstanceID != "" {
		clearGuestAgentWarmup(ctx, st, data.InstanceID)
		_ = st.SetInstanceState(ctx, data.InstanceID, "deleted")
	}
	clearVFPasswordResetCache(ctx, st, data.InstanceID)
	return nil
}

func handleStop(ctx context.Context, st *store.Store, hv hypervisor.Adapter, payload json.RawMessage) error {
	var data struct {
		InstanceID string `json:"instance_id"`
		ExternalID string `json:"external_id"`
		UserID     string `json:"user_id"`
		Reason     string `json:"reason"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	if data.InstanceID == "" {
		return nil
	}
	provider, _, externalID, err := st.GetInstanceProvider(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if store.IsDedicatedProvider(provider) {
		return nil
	}
	if externalID == "" {
		externalID = strings.TrimSpace(data.ExternalID)
	}
	if externalID == "" {
		return nil
	}
	stopFn := hv.StopServer
	if billingStopReason(data.Reason) {
		if err := enforceBillingPowerOff(ctx, hv, externalID); err != nil {
			return err
		}
		return st.MarkInstanceStopped(ctx, data.InstanceID)
	}
	if err := stopFn(ctx, externalID); err != nil {
		return err
	}
	return st.MarkInstanceStopped(ctx, data.InstanceID)
}

func billingStopReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "charge_suspend", "grace_expired", "auto_renew_off", "reconcile_suspend":
		return true
	default:
		return false
	}
}

func handleStart(ctx context.Context, st *store.Store, hv hypervisor.Adapter, payload json.RawMessage) error {
	var data struct {
		InstanceID string `json:"instance_id"`
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	if data.InstanceID == "" {
		return nil
	}
	provider, _, externalID, err := st.GetInstanceProvider(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if store.IsDedicatedProvider(provider) {
		return nil
	}
	if externalID == "" {
		externalID = strings.TrimSpace(data.ExternalID)
	}
	if externalID == "" {
		return nil
	}
	if err := hv.UnsuspendServer(ctx, externalID); err != nil {
		log.Printf("start unsuspend vf=%s: %v", externalID, err)
	}
	if err := hv.StartServer(ctx, externalID); err != nil {
		return err
	}
	return st.SetInstanceRunning(ctx, data.InstanceID)
}

// Give guest-agent time to start after OS build before the first resetPassword call.
func processReinstalling(ctx context.Context, st *store.Store, hv hypervisor.Adapter) error {
	items, err := st.ClaimReinstallingForPoll(ctx, workerID(), 10)
	if err != nil {
		return err
	}
	runReinstallPolls(ctx, st, hv, items, envInt("VPS_PROVISION_WORKERS", 4))
	return nil
}

func pollReinstallingInstance(ctx context.Context, st *store.Store, hv hypervisor.Adapter, item store.CreatingInstance) {
	defer func() {
		if err := st.ReleaseCreatingPollClaim(ctx, item.ID); err != nil {
			log.Printf("release reinstall poll claim %s: %v", item.ID, err)
		}
	}()
	ready, err := hv.EnsureServerOS(ctx, item.ExternalID, item.OSTemplateID, item.RootPassword, nil)
	if err != nil {
		log.Printf("reinstall os %s: %v", item.ID, err)
		handleEnsureOSError(ctx, st, hv, item, err, notify.FailReinstall)
		return
	}
	if !ready {
		log.Printf("reinstall waiting %s (%s)", item.Hostname, item.OSTemplateID)
		return
	}
	ip := ""
	if server, err := hv.GetServer(ctx, item.ExternalID); err == nil {
		ip = server.IP
		if ip != "" {
			if err := st.SetInstanceIP(ctx, item.ID, ip); err != nil {
				log.Printf("reinstall early ip %s: %v", item.ID, err)
			}
			startGuestAgentWarmupClock(ctx, st, item.ID)
		}
	}
	if bootstrapPasswordWorks(ctx, item.RootPassword, ip, item.OSTemplateID) {
		// skip warmup — password already works
	} else if !guestAgentWarmupReady(ctx, st, item.ID) {
		log.Printf("reinstall guest-agent warmup %s (%s)", item.ID, item.OSTemplateID)
		return
	}
	if ip == "" {
		if server, err := hv.GetServer(ctx, item.ExternalID); err == nil {
			ip = server.IP
		}
	}
	password, err := syncRootPassword(ctx, st, hv, item.ID, item.ExternalID, ip, item.RootPassword, item.OSTemplateID)
	if err != nil {
		log.Printf("reinstall password %s: %v", item.ID, err)
		handlePasswordSyncError(ctx, st, hv, item, err, notify.FailReinstall)
		return
	}
	item.RootPassword = password
	ensureQemuGuestAgent(ctx, ip, password, item.OSTemplateID)
	if err := finalizeGuestSSHAccess(ctx, st, item, ip, password); err != nil {
		log.Printf("reinstall finalize %s: %v", item.ID, err)
		handlePasswordSyncError(ctx, st, hv, item, err, notify.FailReinstall)
		return
	}
	if err := st.CompleteReinstall(ctx, item.ID); err != nil {
		log.Printf("reinstall complete %s: %v", item.ID, err)
		return
	}
	if err := applySoftwareProfile(ctx, st, item, ip, password); err != nil {
		log.Printf("reinstall software %s (%s): %v", item.ID, item.SoftwareProfileID, err)
	}
	_ = st.MarkReinstallOutboxPublished(ctx, item.ID)
	clearGuestAgentWarmup(ctx, st, item.ID)
	clearVFPasswordResetCache(ctx, st, item.ID)
	orderNo, _ := st.GetInstanceOrderNumber(ctx, item.ID)
	host := item.Hostname
	if host == "" {
		host = item.ID
	}
	if item.UserID != "" {
		includeSSH := !catalog.IsWindowsOS(item.OSTemplateID)
		if err := notify.InstanceReadyEmail(ctx, item.UserID, host, ip, password, orderNo, notify.ReadyReinstall, includeSSH); err != nil {
			log.Printf("reinstall notify %s: %v", item.ID, err)
		}
	}
}

func provisionSSHKeysForBuild(ctx context.Context, st *store.Store, item store.CreatingInstance) []string {
	keys, err := st.GetInstanceProvisionSSHKeys(ctx, item.ID)
	if err == nil && len(keys) > 0 {
		return buildSSHKeys(item.OSTemplateID, keys)
	}
	return buildSSHKeys(item.OSTemplateID, nil)
}

func handlePollBuildFailure(ctx context.Context, st *store.Store, hv hypervisor.Adapter, item store.CreatingInstance, externalID string, err error, backoff *pollBackoffTracker) {
	log.Printf("retry build %s vf=%s: %v", item.ID, externalID, err)
	if hypervisor.IsIPPoolExhausted(err) {
		_ = requeueForIPPool(ctx, st, hv, item.ID, externalID)
		return
	}
	if hypervisor.IsPermanentProvisionError(err) {
		finalizeFailedCreating(ctx, st, hv, item, err.Error(), notify.FailProvision)
		if backoff != nil {
			backoff.clear(item.ID)
		}
		return
	}
	if backoff != nil {
		backoff.recordFailure(item.ID)
	}
}

func processCreating(ctx context.Context, st *store.Store, hv hypervisor.Adapter, mockMode bool, backoff *pollBackoffTracker) error {
	items, err := st.ClaimCreatingForPoll(ctx, workerID(), 10)
	if err != nil {
		return err
	}
	runCreatingPolls(ctx, st, hv, mockMode, backoff, items, envInt("VPS_PROVISION_WORKERS", 4))
	return nil
}

func runCreatingPolls(ctx context.Context, st *store.Store, hv hypervisor.Adapter, mockMode bool, backoff *pollBackoffTracker, items []store.CreatingInstance, workers int) {
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, item := range items {
		if backoff != nil && backoff.blocked(item.ID) {
			if err := st.ReleaseCreatingPollClaim(ctx, item.ID); err != nil {
				log.Printf("release backoff poll claim %s: %v", item.ID, err)
			}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(item store.CreatingInstance) {
			defer wg.Done()
			defer func() { <-sem }()
			pollCreatingInstance(ctx, st, hv, mockMode, backoff, item)
		}(item)
	}
	wg.Wait()
}

func runReinstallPolls(ctx context.Context, st *store.Store, hv hypervisor.Adapter, items []store.CreatingInstance, workers int) {
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(item store.CreatingInstance) {
			defer wg.Done()
			defer func() { <-sem }()
			pollReinstallingInstance(ctx, st, hv, item)
		}(item)
	}
	wg.Wait()
}

func pollCreatingInstance(ctx context.Context, st *store.Store, hv hypervisor.Adapter, mockMode bool, backoff *pollBackoffTracker, item store.CreatingInstance) {
	defer func() {
		if err := st.ReleaseCreatingPollClaim(ctx, item.ID); err != nil {
			log.Printf("release creating poll claim %s: %v", item.ID, err)
		}
	}()
	if item.ExternalID == "" {
		if mockMode {
			ip := hypervisor.MockIPFromInstance(item.ID)
			if err := finishProvisioning(ctx, st, hv, item, item.ID, ip); err != nil {
				log.Printf("fallback provision %s: %v", item.ID, err)
			}
			return
		}
		opts := provisionCreateOpts(ctx, st, item.NodeID, item.PlanID, item.Region, item.Hostname, item.OSTemplateID, item.RootPassword, provisionSSHKeysForBuild(ctx, st, item))
		externalID, ok, err := ensureHypervisorServerAllocated(ctx, st, hv, item.ID, opts)
		if err != nil {
			log.Printf("retry allocate %s: %v", item.ID, err)
			if hypervisor.IsPermanentProvisionError(err) {
				finalizeFailedCreating(ctx, st, hv, item, err.Error(), notify.FailProvision)
			}
			return
		}
		if !ok {
			return
		}
		if err := hv.BuildServer(ctx, externalID, opts); err != nil {
			handlePollBuildFailure(ctx, st, hv, item, externalID, err, backoff)
		}
		return
	}

	server, err := hv.GetServer(ctx, item.ExternalID)
	if err != nil {
		if hypervisor.IsServerNotFound(err) {
			if clearErr := st.ClearInstanceExternalID(ctx, item.ID); clearErr != nil {
				log.Printf("clear orphan external_id %s: %v", item.ID, clearErr)
			} else {
				log.Printf("cleared orphan external_id for %s (openstack server missing)", item.ID)
			}
		} else {
			log.Printf("poll server %s: %v", item.ID, err)
		}
		return
	}
	needsBuild := hypervisor.ServerNeedsBuild(server)
	if needsBuild {
		opts := provisionCreateOpts(ctx, st, item.NodeID, item.PlanID, item.Region, item.Hostname, item.OSTemplateID, item.RootPassword, provisionSSHKeysForBuild(ctx, st, item))
		if err := hv.BuildServer(ctx, item.ExternalID, opts); err != nil {
			handlePollBuildFailure(ctx, st, hv, item, item.ExternalID, err, backoff)
		}
		return
	}
	if !hypervisor.ServerReadyForGuestSetup(server) {
		return
	}
	if server.HasRunningTasks {
		return
	}
	if err := st.SetInstanceIP(ctx, item.ID, server.IP); err != nil {
		log.Printf("provision early ip %s: %v", item.ID, err)
	}
	// Overlap guest-agent warmup with plan sync / OS ensure (saves ~30–75s wall time).
	startGuestAgentWarmupClock(ctx, st, item.ID)
	if item.PlanID != "" {
		if err := hv.SyncServerPlan(ctx, item.ExternalID, item.PlanID); err != nil {
			log.Printf("sync plan %s: %v", item.ID, err)
		}
	}
	if item.OSTemplateID != "" {
		ready, err := hv.EnsureServerOS(ctx, item.ExternalID, item.OSTemplateID, item.RootPassword, nil)
		if err != nil {
			log.Printf("ensure os %s: %v", item.ID, err)
			handleEnsureOSError(ctx, st, hv, item, err, notify.FailProvision)
		}
		if !ready {
			return
		}
	}
	if bootstrapPasswordWorks(ctx, item.RootPassword, server.IP, item.OSTemplateID) {
		if err := finishProvisioning(ctx, st, hv, item, item.ExternalID, server.IP); err != nil {
			log.Printf("complete provision %s: %v", item.ID, err)
			if strings.Contains(err.Error(), "sync root password") {
				handlePasswordSyncError(ctx, st, hv, item, err, notify.FailProvision)
			}
		} else if backoff != nil {
			backoff.clear(item.ID)
		}
		return
	}
	if !guestAgentWarmupReady(ctx, st, item.ID) {
		log.Printf("provision guest-agent warmup %s (%s)", item.ID, item.OSTemplateID)
		return
	}
	if err := finishProvisioning(ctx, st, hv, item, item.ExternalID, server.IP); err != nil {
		log.Printf("complete provision %s: %v", item.ID, err)
		if strings.Contains(err.Error(), "sync root password") {
			handlePasswordSyncError(ctx, st, hv, item, err, notify.FailProvision)
		}
	} else if backoff != nil {
		backoff.clear(item.ID)
	}
}

func finishProvisioning(ctx context.Context, st *store.Store, hv hypervisor.Adapter, item store.CreatingInstance, externalID, ip string) error {
	if !guestAgentWarmupReady(ctx, st, item.ID) {
		return nil
	}
	if ip != "" {
		if err := st.SetInstanceIP(ctx, item.ID, ip); err != nil {
			log.Printf("provision ip sync %s: %v", item.ID, err)
		}
	}
	if item.PlanID != "" {
		if err := hv.SyncServerPlan(ctx, externalID, item.PlanID); err != nil {
			log.Printf("sync plan %s: %v", item.ID, err)
		}
	}

	if item.OSTemplateID != "" {
		ready, err := hv.EnsureServerOS(ctx, externalID, item.OSTemplateID, item.RootPassword, nil)
		if err != nil {
			log.Printf("ensure os %s: %v", item.ID, err)
			handleEnsureOSError(ctx, st, hv, item, err, notify.FailProvision)
		}
		if !ready {
			return nil
		}
	}

	password, err := syncRootPassword(ctx, st, hv, item.ID, externalID, ip, item.RootPassword, item.OSTemplateID)
	if err != nil {
		// Keep polling — password must be confirmed before credentials are shown.
		return fmt.Errorf("sync root password: %w", err)
	}
	item.RootPassword = password

	ensureQemuGuestAgent(ctx, ip, password, item.OSTemplateID)
	if err := finalizeGuestSSHAccess(ctx, st, item, ip, password); err != nil {
		return err
	}
	if catalog.ResolveOSFamily(item.OSTemplateID) == "windows" && ip != "" {
		if err := winfix.ApplyShellAppsFix(ctx, ip, password, item.OSTemplateID); err != nil {
			log.Printf("windows shell fix %s: %v (continuing)", item.ID, err)
		}
	}

	changed, err := st.CompleteProvisioningIfCreating(ctx, item.ID, externalID, ip, item.NodeID)
	if err != nil {
		return err
	}
	if changed {
		clearGuestAgentWarmup(ctx, st, item.ID)
		clearVFPasswordResetCache(ctx, st, item.ID)
		_ = st.MarkProvisionOutboxPublished(ctx, item.ID)
	}
	if changed && item.UserID != "" {
		orderNo, _ := st.GetInstanceOrderNumber(ctx, item.ID)
		includeSSH := !catalog.IsWindowsOS(item.OSTemplateID)
		if err := notify.InstanceReadyEmail(ctx, item.UserID, item.Hostname, ip, item.RootPassword, orderNo, notify.ReadyProvision, includeSSH); err != nil {
			log.Printf("provision notify %s: %v", item.ID, err)
		}
	}
	if !changed {
		return nil
	}
	// Mark running first; software stacks install in background (retries via VPS_SOFTWARE_RETRY_INTERVAL).
	if err := applySoftwareProfile(ctx, st, item, ip, password); err != nil {
		log.Printf("software install %s (%s): %v", item.ID, item.SoftwareProfileID, err)
	}
	return nil
}

func syncRootPassword(ctx context.Context, st *store.Store, hv hypervisor.Adapter, instanceID, externalID, ip, bootstrap, osTemplateID string) (string, error) {
	bootstrap = strings.TrimSpace(bootstrap)
	isWindows := catalog.ResolveOSFamily(osTemplateID) == "windows"
	const maxResetAttempts = 1
	const maxVerifyAttempts = 8

	loginUser := catalog.ResolvePasswordResetUser(osTemplateID)
	if bootstrap != "" && ip != "" {
		checkErr := sshavail.CheckUserPassword(ctx, ip, loginUser, bootstrap)
		if checkErr == nil {
			log.Printf("ssh verify %s: bootstrap password accepted before VF reset", instanceID)
			if err := st.UpdateInstanceRootPassword(ctx, instanceID, bootstrap); err != nil {
				return "", err
			}
			return bootstrap, nil
		}
		if !isWindows {
			if err := sshavail.CheckRootPassword(ctx, ip, bootstrap); err == nil {
				log.Printf("ssh verify %s: bootstrap root password accepted before VF reset", instanceID)
				if err := st.UpdateInstanceRootPassword(ctx, instanceID, bootstrap); err != nil {
					return "", err
				}
				return bootstrap, nil
			}
		}
	}

	var lastErr error
	var password string
	resetAttempts := 0
	for attempt := 1; attempt <= maxVerifyAttempts; attempt++ {
		if password == "" {
			if resetAttempts >= maxResetAttempts {
				break
			}
			if !vfPasswordResetAllowed(ctx, st, instanceID) {
				lastErr = fmt.Errorf("openstack: resetPassword cooldown (guest agent warmup)")
				if err := sleepCtx(ctx, 8*time.Second); err != nil {
					return "", err
				}
				continue
			}
			resetAttempts++
			markVFPasswordReset(ctx, st, instanceID)
			pwd, err := hv.ResetRootPassword(ctx, externalID, osTemplateID)
			if err != nil {
				lastErr = err
				log.Printf("reset root password %s attempt %d: %v", instanceID, resetAttempts, err)
				if isWindows && bootstrap != "" && hypervisor.IsNotCommissionedError(err) {
					log.Printf("reset root password %s: vf not commissioned, using order bootstrap password", instanceID)
					if err := st.UpdateInstanceRootPassword(ctx, instanceID, bootstrap); err != nil {
						return "", err
					}
					return bootstrap, nil
				}
				if hypervisor.IsGuestAgentUnavailable(err) {
					log.Printf("reset root password %s: guest agent unavailable on hypervisor (will verify expectedPassword via SSH)", instanceID)
				}
				if hypervisor.IsHypervisorJobFailed(err) {
					if err := sleepCtx(ctx, 4*time.Second); err != nil {
						return "", err
					}
					continue
				}
				if err := sleepCtx(ctx, 4*time.Second); err != nil {
					return "", err
				}
				continue
			}
			password = strings.TrimSpace(pwd)
			if password == "" {
				lastErr = fmt.Errorf("hypervisor returned an empty password")
				if err := sleepCtx(ctx, 4*time.Second); err != nil {
					return "", err
				}
				continue
			}
			if ip != "" && !isWindows {
				ensureQemuGuestAgent(ctx, ip, password, osTemplateID)
			}
			if bootstrap != "" && ip != "" {
				if applied, err := sshavail.ApplyDesiredPassword(ctx, ip, loginUser, password, bootstrap, isWindows); err == nil {
					password = applied
				} else {
					log.Printf("apply bootstrap password %s (%s): %v", instanceID, ip, err)
				}
			}
		}

		if ip == "" || hypervisor.MockEnabled() {
			if err := st.UpdateInstanceRootPassword(ctx, instanceID, password); err != nil {
				return "", err
			}
			return password, nil
		}
		if isWindows {
			if err := st.UpdateInstanceRootPassword(ctx, instanceID, password); err != nil {
				return "", err
			}
			return password, nil
		}

		if err := sshavail.CheckRootPassword(ctx, ip, password); err != nil {
			lastErr = err
			log.Printf("ssh verify %s attempt %d: %v", instanceID, attempt, err)
			// Keys-only guests: password auth is disabled — finish as soon as SSH is listening.
			if sshavail.TCPOpen(ctx, ip, 22) && sshavail.PasswordAuthDisabled(err) {
				log.Printf("ssh verify %s: password auth disabled (SSH keys); accepting VirtFusion password", instanceID)
				if err := st.UpdateInstanceRootPassword(ctx, instanceID, password); err != nil {
					return "", err
				}
				return password, nil
			}
			// Keep the same password — do not reset again (each reset costs ~8s+ and churns the guest).
			if err := sleepCtx(ctx, 4*time.Second); err != nil {
				return "", err
			}
			continue
		}

		if err := stabilizeSSHHostKeys(ctx, st, ip, password); err != nil {
			log.Printf("stabilize ssh host keys %s (%s): %v", instanceID, ip, err)
			// Non-fatal for first boot; password still works.
		}
		if err := st.UpdateInstanceRootPassword(ctx, instanceID, password); err != nil {
			return "", err
		}
		return password, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("password not accepted by ssh")
	}
	if bootstrap != "" && ip != "" {
		if err := sshavail.CheckUserPassword(ctx, ip, loginUser, bootstrap); err == nil {
			log.Printf("ssh verify %s: bootstrap password accepted after VF sync failure", instanceID)
			if err := st.UpdateInstanceRootPassword(ctx, instanceID, bootstrap); err != nil {
				return "", err
			}
			return bootstrap, nil
		}
		if isWindows && hypervisor.IsNotCommissionedError(lastErr) {
			log.Printf("ssh verify %s: vf not commissioned, using order bootstrap password", instanceID)
			if err := st.UpdateInstanceRootPassword(ctx, instanceID, bootstrap); err != nil {
				return "", err
			}
			return bootstrap, nil
		}
		if !isWindows {
			if err := sshavail.CheckRootPassword(ctx, ip, bootstrap); err == nil {
				log.Printf("ssh verify %s: bootstrap root password accepted after VF sync failure", instanceID)
				if err := st.UpdateInstanceRootPassword(ctx, instanceID, bootstrap); err != nil {
					return "", err
				}
				return bootstrap, nil
			}
		}
	}
	return "", lastErr
}

func completeInstancePostPassword(ctx context.Context, st *store.Store, item store.CreatingInstance, ip, password string) error {
	ensureQemuGuestAgent(ctx, ip, password, item.OSTemplateID)
	return finalizeGuestSSHAccess(ctx, st, item, ip, password)
}

func applySoftwareProfile(ctx context.Context, st *store.Store, item store.CreatingInstance, ip, password string) error {
	profile := strings.TrimSpace(item.SoftwareProfileID)
	if profile == "" || profile == "clean" {
		return nil
	}
	if hypervisor.MockEnabled() {
		log.Printf("software install %s: skipped in mock mode (%s)", item.ID, profile)
		return nil
	}
	if catalog.ResolveOSFamily(item.OSTemplateID) == "windows" {
		log.Printf("software install %s: skip %s on windows", item.ID, profile)
		return nil
	}

	const maxSoftwareAttempts = 5
	var bundle *softinstall.Bundle
	var installErr error
	for attempt := 1; attempt <= maxSoftwareAttempts; attempt++ {
		if err := waitSSHRootForSoftware(ctx, ip, password); err != nil {
			installErr = err
			if attempt == maxSoftwareAttempts || !isTransientSSHInstallErr(err) {
				break
			}
			log.Printf("software install %s (%s) ssh wait attempt %d: %v", item.ID, profile, attempt, err)
			if err := sleepCtx(ctx, 5*time.Second); err != nil {
				return err
			}
			continue
		}
		bundle, installErr = softinstall.Apply(ctx, ip, password, item.OSTemplateID, profile)
		if installErr == nil {
			break
		}
		if attempt == maxSoftwareAttempts || !isTransientSSHInstallErr(installErr) {
			break
		}
		log.Printf("software install %s (%s) attempt %d: %v", item.ID, profile, attempt, installErr)
		if err := sleepCtx(ctx, 5*time.Second); err != nil {
			return err
		}
	}
	if installErr != nil {
		if st != nil {
			_ = st.SetInstanceProviderMeta(ctx, item.ID, map[string]any{
				"software_install_error": installErr.Error(),
				"software_profile_id":    profile,
			})
		}
		return installErr
	}
	if st != nil {
		meta := map[string]any{
			"software_profile_id": profile,
		}
		if bundle != nil {
			meta["software_bundle"] = softinstall.BundleMap(bundle)
		}
		if err := st.CompleteSoftwareInstallMeta(ctx, item.ID, meta); err != nil {
			log.Printf("software bundle save %s: %v", item.ID, err)
		}
	}
	return nil
}

func waitSSHRootForSoftware(ctx context.Context, ip, password string) error {
	const maxAttempts = 6
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := sshavail.CheckRootPassword(ctx, ip, password); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == maxAttempts {
			break
		}
		if err := sleepCtx(ctx, 4*time.Second); err != nil {
			return err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ssh not ready")
	}
	return fmt.Errorf("ssh not ready for software install: %w", lastErr)
}

func isTransientSSHInstallErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "handshake failed") ||
		strings.Contains(s, "auth failed") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "ssh not ready for software install")
}

// buildSSHKeys returns keys for VirtFusion build. On Linux we defer keys until after
// software preinstall so password SSH stays available; Windows keeps VF key injection.
// Invalid keys are dropped so they cannot block OS install.
func buildSSHKeys(osTemplateID string, keys []string) []string {
	valid, skipped := sshpubkey.FilterValid(keys)
	if skipped > 0 {
		log.Printf("provision: dropped %d invalid ssh public key(s)", skipped)
	}
	if catalog.ResolveOSFamily(osTemplateID) == "windows" {
		return valid
	}
	return nil
}

func finalizeGuestSSHAccess(ctx context.Context, st *store.Store, item store.CreatingInstance, ip, password string) error {
	if catalog.ResolveOSFamily(item.OSTemplateID) == "windows" {
		return nil
	}
	keys, err := st.GetInstanceProvisionSSHKeys(ctx, item.ID)
	if err != nil || len(keys) == 0 {
		keys = loadUserSSHKeys(ctx, st, item.UserID)
	}
	keys, skipped := sshpubkey.FilterValid(keys)
	if skipped > 0 {
		log.Printf("ssh keys install: skipped %d invalid key(s) for %s", skipped, item.ID)
	}
	if pub := opsssh.AuthorizedKeyLine(); pub != "" {
		keys = appendUniqueSSHKey(keys, pub)
	}
	if len(keys) == 0 {
		return nil
	}
	// Install user keys; keep password auth enabled so platform automation (IP change) still works.
	if err := sshavail.InstallAuthorizedKeys(ctx, ip, password, keys, false); err != nil {
		log.Printf("ssh keys install soft-fail %s: %v (continuing with password auth)", item.ID, err)
		return nil
	}
	log.Printf("ssh keys installed for %s (%d keys, password auth kept)", item.ID, len(keys))
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func appendUniqueSSHKey(keys []string, pub string) []string {
	pub = strings.TrimSpace(pub)
	if pub == "" {
		return keys
	}
	for _, k := range keys {
		if strings.TrimSpace(k) == pub {
			return keys
		}
	}
	return append(keys, pub)
}

// stabilizeSSHHostKeys keeps a stable host key per recycled public IP so clients
// do not hit "REMOTE HOST IDENTIFICATION HAS CHANGED" after delete+create.
func stabilizeSSHHostKeys(ctx context.Context, st *store.Store, ip, password string) error {
	stored, err := st.GetIPSSHHostKeys(ctx, ip)
	if err != nil {
		return err
	}
	if stored != nil && stored.Ed25519Private != "" {
		if err := sshavail.InstallHostKeys(ctx, ip, password, sshavail.HostKeyMaterial{
			Ed25519Private: stored.Ed25519Private,
			Ed25519Public:  stored.Ed25519Public,
			ECDSAPrivate:   stored.ECDSAPrivate,
			ECDSAPublic:    stored.ECDSAPublic,
		}); err != nil {
			return err
		}
		// Confirm SSH still works after sshd restart with restored keys.
		return sshavail.CheckRootPassword(ctx, ip, password)
	}

	live, err := sshavail.ReadHostKeys(ctx, ip, password)
	if err != nil {
		return err
	}
	return st.UpsertIPSSHHostKeys(ctx, store.IPSShHostKeys{
		IP:             ip,
		Ed25519Private: live.Ed25519Private,
		Ed25519Public:  live.Ed25519Public,
		ECDSAPrivate:   live.ECDSAPrivate,
		ECDSAPublic:    live.ECDSAPublic,
	})
}

func outboxInstanceID(payload json.RawMessage) string {
	var data struct {
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.InstanceID)
}

func outboxHostname(payload json.RawMessage) string {
	var data struct {
		Hostname string `json:"hostname"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Hostname)
}

func provisionCreateOpts(ctx context.Context, st *store.Store, nodeID, planID, region, hostname, osTemplate, password string, sshKeys []string) hypervisor.CreateOptions {
	opts := hypervisor.CreateOptions{
		PlanID:       planID,
		Region:       region,
		Hostname:     hostname,
		OSTemplateID: osTemplate,
		RootPassword: password,
		SSHKeys:      sshKeys,
	}
	if cr, err := st.GetNodeComputeResourceID(ctx, nodeID); err == nil && cr > 0 {
		opts.ComputeResourceID = cr
	}
	if nodeID != "" {
		opts.NodeID = nodeID
		if ext, err := st.GetNodeExternalID(ctx, nodeID); err == nil {
			opts.HypervisorExternalID = ext
		}
	}
	return opts
}

func loadUserSSHKeys(ctx context.Context, st *store.Store, userID string) []string {
	keys, err := st.ListAllSSHPublicKeys(ctx, userID)
	if err != nil {
		log.Printf("load ssh keys for %s: %v", userID, err)
		return nil
	}
	return keys
}

// VirtFusion can flicker buildFailed=true for a few seconds mid-rebuild.
// Only promote to instance "error" after the install has been stuck long enough.
const ensureOSFailAfter = 15 * time.Minute

// Stop hammering VF resetPassword when guest agent is dead (queue jobs fail instantly).
const passwordQueueFailAfter = 5 * time.Minute
const passwordSyncFailAfter = 15 * time.Minute

func handlePasswordSyncError(ctx context.Context, st *store.Store, hv hypervisor.Adapter, item store.CreatingInstance, err error, kind notify.FailKind) {
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "bootstrap password not ready") ||
		strings.Contains(msg, "resetPassword cooldown") ||
		strings.Contains(msg, "guest agent warmup") ||
		hypervisor.IsGuestAgentUnavailable(err) ||
		hypervisor.IsNotCommissionedError(err) {
		log.Printf("password sync %s: transient %v", item.ID, err)
		return
	}
	deadline := passwordSyncFailAfter
	if hypervisor.IsHypervisorJobFailed(err) {
		deadline = passwordQueueFailAfter
	}
	anchor := item.UpdatedAt
	if phaseStart := passwordSyncPhaseStart(ctx, st, item.ID); !phaseStart.IsZero() {
		anchor = phaseStart
	}
	if !anchor.IsZero() && time.Since(anchor) < deadline {
		log.Printf("password sync %s: %v (fail after %s)", item.ID, err, (deadline - time.Since(anchor)).Round(time.Second))
		return
	}
	finalizeFailedCreating(ctx, st, hv, item, err.Error(), kind)
}

func handleEnsureOSError(ctx context.Context, st *store.Store, hv hypervisor.Adapter, item store.CreatingInstance, err error, kind notify.FailKind) {
	if err == nil || !strings.Contains(err.Error(), "os sync failed") {
		return
	}
	since := item.UpdatedAt
	if !since.IsZero() && time.Since(since) < ensureOSFailAfter {
		log.Printf("ensure os %s: transient %v (wait %s before fail)", item.ID, err, ensureOSFailAfter-time.Since(since).Round(time.Second))
		return
	}
	finalizeFailedCreating(ctx, st, hv, item, err.Error(), kind)
}

func refreshMetrics(ctx context.Context, st *store.Store, hv hypervisor.Adapter) error {
	batch := envInt("VPS_METRICS_BATCH", 15)
	if batch <= 0 {
		batch = 15
	}
	ids, err := st.ListRunningInstanceIDs(ctx, batch)
	if err != nil {
		return err
	}
	var failed int
	for _, id := range ids {
		externalID, err := st.GetInstanceExternalID(ctx, id)
		if err != nil {
			continue
		}
		metrics, err := hv.GetMetrics(ctx, externalID)
		if err != nil {
			if hypervisor.IsServerNotFound(err) || isMetricsUnavailable(err) {
				continue
			}
			failed++
			if failed <= 3 {
				log.Printf("vps worker metrics %s ext=%s: %v", id, externalID, err)
			}
			continue
		}
		b, _ := json.Marshal(metrics)
		if err := st.UpdateInstanceMetrics(ctx, id, b); err != nil {
			log.Printf("vps worker metrics store %s: %v", id, err)
		}
	}
	if failed > 0 {
		log.Printf("vps worker metrics: %d/%d failed (check VF /servers/{id}/metrics API)", failed, len(ids))
	}
	return nil
}

func isMetricsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "404") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "/metrics")
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
