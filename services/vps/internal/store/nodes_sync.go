package store

import (
	"context"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

type NodeSyncTarget struct {
	ID         string
	ExternalID string
}

type NodeSyncPatch struct {
	NodeID            string
	Status            string
	VFName            string
	VFIP              string
	VFHostname        string
	VFEnabled         bool
	MaintenanceMode   bool
	VFCommissioned    int
	MaxCPUCores       int
	CPUAllocated      int
	CPUUsedPercent    float64
	MaxMemoryMB       int
	MemoryAllocatedMB int
	MemoryUsedPercent float64
	MaxDiskGB         int
	DiskAllocatedGB   int
	DiskUsedPercent   float64
	VFServerCount     int
	CapacityInstances *int
	LastSyncedAt      time.Time
}

func (s *Store) ListNodesForSync(ctx context.Context) ([]NodeSyncTarget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, external_id
		FROM vps.nodes
		WHERE external_id IS NOT NULL AND TRIM(external_id) <> ''
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []NodeSyncTarget
	for rows.Next() {
		var row NodeSyncTarget
		if err := rows.Scan(&row.ID, &row.ExternalID); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) ApplyNodeSync(ctx context.Context, patch NodeSyncPatch) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.nodes SET
			status = $2,
			vf_name = NULLIF($3, ''),
			vf_ip = NULLIF($4, ''),
			vf_hostname = NULLIF($5, ''),
			vf_enabled = $6,
			maintenance_mode = $7,
			vf_commissioned = NULLIF($8, 0),
			max_cpu_cores = NULLIF($9, 0),
			cpu_allocated = NULLIF($10, 0),
			cpu_used_percent = $11,
			max_memory_mb = NULLIF($12, 0),
			memory_allocated_mb = NULLIF($13, 0),
			memory_used_percent = $14,
			max_disk_gb = NULLIF($15, 0),
			disk_allocated_gb = NULLIF($16, 0),
			disk_used_percent = $17,
			vf_server_count = NULLIF($18, 0),
			capacity_instances = COALESCE($19, capacity_instances),
			last_synced_at = $20,
			updated_at = now()
		WHERE id = $1::uuid
	`,
		patch.NodeID,
		patch.Status,
		patch.VFName,
		patch.VFIP,
		patch.VFHostname,
		patch.VFEnabled,
		patch.MaintenanceMode,
		patch.VFCommissioned,
		patch.MaxCPUCores,
		patch.CPUAllocated,
		patch.CPUUsedPercent,
		patch.MaxMemoryMB,
		patch.MemoryAllocatedMB,
		patch.MemoryUsedPercent,
		patch.MaxDiskGB,
		patch.DiskAllocatedGB,
		patch.DiskUsedPercent,
		patch.VFServerCount,
		patch.CapacityInstances,
		patch.LastSyncedAt,
	)
	return err
}

func (s *Store) MarkNodeSyncMissing(ctx context.Context, nodeID string, syncedAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.nodes SET
			status = 'offline',
			vf_enabled = false,
			last_synced_at = $2,
			updated_at = now()
		WHERE id = $1::uuid
	`, nodeID, syncedAt)
	return err
}

func ApplyReachability(patch *NodeSyncPatch, reachable bool) {
	if patch == nil {
		return
	}
	if reachable || strings.TrimSpace(patch.VFIP) == "" {
		return
	}
	patch.Status = "offline"
}

func PatchFromHypervisor(nodeID string, snap hypervisor.ComputeSnapshot, syncedAt time.Time) NodeSyncPatch {
	patch := NodeSyncPatch{
		NodeID:            nodeID,
		Status:            hypervisor.DeriveNodeStatus(snap),
		VFName:            snap.Name,
		VFIP:              snap.IP,
		VFHostname:        snap.Hostname,
		VFEnabled:         snap.Enabled,
		MaintenanceMode:   snap.Maintenance,
		VFCommissioned:    snap.Commissioned,
		MaxCPUCores:       snap.MaxCPU,
		CPUAllocated:      snap.CPUAllocated,
		CPUUsedPercent:    snap.CPUUsedPercent,
		MaxMemoryMB:       snap.MaxMemoryMB,
		MemoryAllocatedMB: snap.MemoryAllocatedMB,
		MemoryUsedPercent: snap.MemoryUsedPercent,
		MaxDiskGB:         snap.DiskMaxGB,
		DiskAllocatedGB:   snap.DiskAllocatedGB,
		DiskUsedPercent:   snap.DiskUsedPercent,
		VFServerCount:     snap.ServerCount,
		LastSyncedAt:      syncedAt,
	}
	if snap.MaxServers > 0 {
		patch.CapacityInstances = &snap.MaxServers
	}
	return patch
}
