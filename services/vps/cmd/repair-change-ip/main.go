package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hvinit"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/opsssh"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func main() {
	instanceID := flag.String("instance", "", "portal instance UUID")
	oldIP := flag.String("old-ip", "", "stale primary IPv4 to release")
	newIP := flag.String("new-ip", "", "target primary IPv4")
	reachIP := flag.String("reach-ip", "", "optional live SSH dial IP when target is not up yet")
	gateway := flag.String("gateway", "", "replacement IPv4 gateway")
	prefix := flag.Int("prefix", 24, "IPv4 prefix length")
	refund := flag.Bool("refund", false, "refund one failed IP-change fee")
	flag.Parse()

	if strings.TrimSpace(*instanceID) == "" || strings.TrimSpace(*newIP) == "" || strings.TrimSpace(*gateway) == "" {
		log.Fatal("-instance, -new-ip and -gateway are required")
	}
	dialIP := strings.TrimSpace(*reachIP)
	if dialIP == "" {
		dialIP = strings.TrimSpace(*newIP)
	}
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	if dsn == "" {
		log.Fatal("POSTGRES_DSN is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	st, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	externalID, err := st.GetInstanceExternalID(ctx, *instanceID)
	if err != nil || strings.TrimSpace(externalID) == "" {
		log.Fatalf("external id: %v", err)
	}
	userID, err := st.GetInstanceOwner(ctx, *instanceID)
	if err != nil {
		log.Fatalf("owner: %v", err)
	}
	password, err := st.GetInstanceRootPassword(ctx, *instanceID)
	if err != nil {
		log.Fatalf("password: %v", err)
	}
	auth := sshavail.GuestSSHAuth{Password: password}
	if signer, ok := opsssh.Signer(); ok {
		auth.Signer = signer
	}

	if err := sshavail.CheckRootAuth(ctx, dialIP, auth); err != nil {
		log.Fatalf("dial IP SSH is unavailable (%s): %v", dialIP, err)
	}
	if err := sshavail.ReconfigureStaticIPv4Auth(ctx, dialIP, auth, *newIP, *gateway, *prefix); err != nil {
		log.Fatalf("persist replacement IP: %v", err)
	}
	time.Sleep(5 * time.Second)
	for i := 0; i < 10; i++ {
		if err := sshavail.CheckRootAuth(ctx, *newIP, auth); err == nil {
			break
		}
		if i == 9 {
			log.Fatalf("replacement IP verification: unreachable on %s", *newIP)
		}
		time.Sleep(3 * time.Second)
	}

	hv := hvinit.NewAdapter()
	if old := strings.TrimSpace(*oldIP); old != "" && old != strings.TrimSpace(*newIP) {
		if err := hv.RemovePrimaryIPv4(ctx, externalID, old); err != nil && !hypervisor.IsServerNotFound(err) {
			log.Fatalf("release stale IP %s: %v", old, err)
		}
	}
	if err := hv.SyncServerNetworkFilters(ctx, externalID); err != nil {
		log.Fatalf("network filter sync: %v", err)
	}
	logOpts := &store.IPAssignmentLogOpts{Source: store.IPSourceChangeIP}
	if err := st.SetInstanceIPAddress(ctx, *instanceID, *newIP, logOpts); err != nil {
		log.Fatalf("save replacement IP: %v", err)
	}
	if ips, _, _, err := hv.PrimaryIPv4Info(ctx, externalID); err == nil && len(ips) > 0 {
		if err := st.SetInstanceAllIPs(ctx, *instanceID, ips); err != nil {
			log.Printf("save attached IP list: %v", err)
		}
	}
	if err := st.ClearChangeIPLock(ctx, *instanceID); err != nil {
		log.Fatalf("clear change-IP lock: %v", err)
	}
	if *refund {
		reason := fmt.Sprintf("VPS IP change refund (manual recovery instance %s)", *instanceID)
		if err := st.RefundChangeIPFee(ctx, userID, *instanceID, reason); err != nil {
			log.Fatalf("refund: %v", err)
		}
	}
	log.Printf("repaired instance=%s external=%s ip=%s", *instanceID, externalID, *newIP)
}
