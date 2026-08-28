package hypervisor

import (
	"context"
	"time"
)

type Server struct {
	ID               string
	Status           string
	RealStatus       string
	IP               string
	OSImageVersionID int
	// OSBuilt is true when VirtFusion reports a non-null "built" timestamp (OS on disk).
	OSBuilt          bool
	HasRunningTasks  bool
	BuildFailed      bool
	// GuestAgentActive is true when VF reports a live QEMU guest agent (VM is powered on).
	GuestAgentActive bool
	// Suspended is true when VF account/network suspension is active on the server.
	Suspended bool
	// RemoteStateKnown is true when VF returned remoteState (hypervisor reachability).
	RemoteStateKnown bool
	// RemoteState is true when the VM is running on the hypervisor (per VF remoteState).
	RemoteState bool
}

type CreateOptions struct {
	PlanID              string
	Region              string
	Hostname            string
	OSTemplateID        string
	RootPassword        string
	SSHKeys             []string
	ComputeResourceID   int
	// NodeID is the portal vps.nodes UUID (OpenStack node pinning via OPENSTACK_NODE_MAP).
	NodeID string
	// HypervisorExternalID is vps.nodes.external_id (Nova hypervisor UUID or hostname).
	HypervisorExternalID string
	// ComputeHost is an explicit Nova scheduler host hint (optional).
	ComputeHost string
}

type ConsoleSession struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

type Metrics struct {
	CPUUsagePercent  float64   `json:"cpu_usage_percent"`
	RAMUsagePercent  float64   `json:"ram_usage_percent"`
	DiskUsagePercent float64   `json:"disk_usage_percent"`
	RxMbps           float64   `json:"rx_mbps"`
	TxMbps           float64   `json:"tx_mbps"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Snapshot struct {
	ID     string
	Name   string
	Status string
}

type ComputeSnapshot struct {
	ExternalID        string
	Name              string
	IP                string
	Hostname          string
	Enabled           bool
	Maintenance       bool
	Commissioned      int
	MaxServers        int
	MaxCPU            int
	MaxMemoryMB       int
	CPUAllocated      int
	CPUUsedPercent    float64
	MemoryAllocatedMB int
	MemoryUsedPercent float64
	DiskMaxGB         int
	DiskAllocatedGB   int
	DiskUsedPercent   float64
	ServerCount       int
}

type Adapter interface {
	AllocateServer(ctx context.Context, opts CreateOptions) (*Server, error)
	BuildServer(ctx context.Context, serverID string, opts CreateOptions) error
	CreateServer(ctx context.Context, opts CreateOptions) (*Server, error)
	GetServer(ctx context.Context, id string) (*Server, error)
	StartServer(ctx context.Context, id string) error
	StopServer(ctx context.Context, id string) error
	PowerOffServer(ctx context.Context, id string) error
	SuspendServer(ctx context.Context, id string) error
	UnsuspendServer(ctx context.Context, id string) error
	RebootServer(ctx context.Context, id string) error
	DeleteServer(ctx context.Context, id string) error
	ResizeServer(ctx context.Context, id string, packageID int, preserveDisk bool) error
	SyncServerPlan(ctx context.Context, serverID, catalogPlanID string) error
	// AddPrimaryIPv4 assigns one free address and returns it (old IPs stay attached).
	AddPrimaryIPv4(ctx context.Context, serverID string) (newIP string, err error)
	// AddExtraIPv4 assigns qty additional addresses on the primary interface (existing IPs stay).
	AddExtraIPv4(ctx context.Context, serverID string, qty int) (newIPs []string, err error)
	// RemovePrimaryIPv4 detaches the given address from the primary interface.
	RemovePrimaryIPv4(ctx context.Context, serverID, ip string) error
	// SyncServerNetworkFilters refreshes native ebtables/network-filter rules on the
	// hypervisor after IP assignment changes (no-op when VF ctrl SSH is not configured).
	SyncServerNetworkFilters(ctx context.Context, serverID string) error
	// PrimaryIPv4Info returns current primary IPs plus gateway/netmask from VF.
	PrimaryIPv4Info(ctx context.Context, serverID string) (ips []string, gateway, netmask string, err error)
	// HasFreePrimaryIPv4 probes VF IP blocks before promoting ip_pool waitlist rows.
	HasFreePrimaryIPv4(ctx context.Context, region, hypervisorID string) (bool, error)
	EnsureServerOS(ctx context.Context, serverID, catalogOSTemplateID, rootPassword string, sshKeys []string) (ready bool, err error)
	ReinstallServer(ctx context.Context, id, planID, osTemplateID, rootPassword string, sshKeys []string) error
	ResetRootPassword(ctx context.Context, id, osTemplateID string) (string, error)
	SetRootPassword(ctx context.Context, id, password string) error
	GetConsole(ctx context.Context, id string) (*ConsoleSession, error)
	GetMetrics(ctx context.Context, id string) (*Metrics, error)
	CreateSnapshot(ctx context.Context, id, name string) (*Snapshot, error)
	DeleteSnapshot(ctx context.Context, id, snapshotID string) error
}

type NodeSyncSource interface {
	FetchComputeSnapshots(ctx context.Context) ([]ComputeSnapshot, error)
}

type OSImageRow struct {
	Name      string
	Version   string
	VersionID int
}

type OSSyncSource interface {
	FetchOSImages(ctx context.Context) ([]OSImageRow, error)
}
