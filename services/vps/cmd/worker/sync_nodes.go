package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func syncNodes(ctx context.Context, st *store.Store, src hypervisor.NodeSyncSource) error {
	if src == nil {
		return nil
	}
	targets, err := st.ListNodesForSync(ctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	snapshots, err := src.FetchComputeSnapshots(ctx)
	if err != nil {
		return err
	}
	byExternalID := make(map[string]hypervisor.ComputeSnapshot, len(snapshots))
	byName := make(map[string]hypervisor.ComputeSnapshot, len(snapshots))
	for _, snap := range snapshots {
		byExternalID[snap.ExternalID] = snap
		if name := strings.ToLower(strings.TrimSpace(snap.Name)); name != "" {
			byName[name] = snap
		}
	}

	now := time.Now().UTC()
	for _, target := range targets {
		snap, ok := byExternalID[target.ExternalID]
		if !ok {
			snap, ok = byName[strings.ToLower(strings.TrimSpace(target.ExternalID))]
		}
		if !ok {
			if err := st.MarkNodeSyncMissing(ctx, target.ID, now); err != nil {
				log.Printf("node sync missing %s: %v", target.ID, err)
			}
			continue
		}
		patch := store.PatchFromHypervisor(target.ID, snap, now)
		if host := strings.TrimSpace(snap.IP); host != "" {
			reachable := hypervisor.ProbeTCP(ctx, host, hypervisor.DefaultHypervisorPort)
			store.ApplyReachability(&patch, reachable)
			if !reachable {
				log.Printf("node sync %s (%s): hypervisor unreachable on :%d", target.ID, host, hypervisor.DefaultHypervisorPort)
			}
		}
		if err := st.ApplyNodeSync(ctx, patch); err != nil {
			log.Printf("node sync apply %s: %v", target.ID, err)
		}
	}
	return nil
}
