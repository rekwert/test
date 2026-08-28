package hypervisor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type MockAdapter struct {
	mu      sync.Mutex
	servers map[string]*mockServer
}

type mockServer struct {
	id       string
	status   string
	ip       string
	hostname string
	region   string
	osID     string
	metrics  Metrics
}

func NewMock() *MockAdapter {
	return &MockAdapter{servers: map[string]*mockServer{}}
}

func (m *MockAdapter) AllocateServer(ctx context.Context, opts CreateOptions) (*Server, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	id := randomID()
	m.servers[id] = &mockServer{
		id:       id,
		status:   "queued",
		hostname: opts.Hostname,
		region:   opts.Region,
		osID:     opts.OSTemplateID,
	}
	return &Server{ID: id, Status: "queued"}, nil
}

func (m *MockAdapter) BuildServer(ctx context.Context, serverID string, opts CreateOptions) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverID]
	if !ok {
		s = &mockServer{id: serverID}
		m.servers[serverID] = s
	}
	s.status = "running"
	s.ip = randomIP()
	s.hostname = opts.Hostname
	s.osID = opts.OSTemplateID
	s.metrics = Metrics{
		CPUUsagePercent:  12,
		RAMUsagePercent:  34,
		DiskUsagePercent: 28,
		RxMbps:           1.2,
		TxMbps:           0.8,
		UpdatedAt:        time.Now().UTC(),
	}
	return nil
}

func (m *MockAdapter) CreateServer(ctx context.Context, opts CreateOptions) (*Server, error) {
	server, err := m.AllocateServer(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := m.BuildServer(ctx, server.ID, opts); err != nil {
		return nil, err
	}
	return &Server{ID: server.ID, Status: "running", IP: m.servers[server.ID].ip}, nil
}

func (m *MockAdapter) GetServer(ctx context.Context, id string) (*Server, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[id]
	if !ok {
		return &Server{ID: id, Status: "running", IP: "10.66.0.10"}, nil
	}
	return &Server{ID: s.id, Status: s.status, IP: s.ip}, nil
}

func (m *MockAdapter) StartServer(ctx context.Context, id string) error {
	return m.setStatus(ctx, id, "running")
}

func (m *MockAdapter) StopServer(ctx context.Context, id string) error {
	return m.setStatus(ctx, id, "stopped")
}

func (m *MockAdapter) PowerOffServer(ctx context.Context, id string) error {
	return m.setStatus(ctx, id, "stopped")
}

func (m *MockAdapter) SuspendServer(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.servers[id]; ok {
		s.status = "stopped"
	}
	return nil
}

func (m *MockAdapter) UnsuspendServer(ctx context.Context, id string) error {
	_ = ctx
	return nil
}

func (m *MockAdapter) RebootServer(ctx context.Context, id string) error {
	return m.setStatus(ctx, id, "running")
}

func (m *MockAdapter) DeleteServer(ctx context.Context, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.servers, id)
	return nil
}

func (m *MockAdapter) ResizeServer(ctx context.Context, id string, packageID int, preserveDisk bool) error {
	_ = ctx
	_ = id
	_ = packageID
	_ = preserveDisk
	return nil
}

func (m *MockAdapter) SyncServerPlan(ctx context.Context, serverID, catalogPlanID string) error {
	_ = ctx
	_ = serverID
	_ = catalogPlanID
	return nil
}

func (m *MockAdapter) ChangePrimaryIPv4(ctx context.Context, serverID string) (oldIP, newIP string, err error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverID]
	if !ok {
		return "", "", fmt.Errorf("server not found")
	}
	oldIP = s.ip
	newIP = mockNextIP(oldIP)
	s.ip = newIP
	return oldIP, newIP, nil
}

func (m *MockAdapter) AddPrimaryIPv4(ctx context.Context, serverID string) (string, error) {
	_, newIP, err := m.ChangePrimaryIPv4(ctx, serverID)
	return newIP, err
}

func (m *MockAdapter) AddExtraIPv4(ctx context.Context, serverID string, qty int) ([]string, error) {
	if qty <= 0 {
		return nil, fmt.Errorf("invalid quantity")
	}
	out := make([]string, 0, qty)
	for i := 0; i < qty; i++ {
		ip, err := m.AddPrimaryIPv4(ctx, serverID)
		if err != nil {
			return out, err
		}
		out = append(out, ip)
	}
	return out, nil
}

func (m *MockAdapter) RemovePrimaryIPv4(ctx context.Context, serverID, ip string) error {
	_ = ctx
	_ = serverID
	_ = ip
	return nil
}

func (m *MockAdapter) SyncServerNetworkFilters(ctx context.Context, serverID string) error {
	_ = ctx
	_ = serverID
	return nil
}

func (m *MockAdapter) PrimaryIPv4Info(ctx context.Context, serverID string) ([]string, string, string, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverID]
	if !ok {
		return nil, "", "", fmt.Errorf("server not found")
	}
	return []string{s.ip}, "10.66.0.1", "255.255.255.0", nil
}

func (m *MockAdapter) HasFreePrimaryIPv4(ctx context.Context, region, hypervisorID string) (bool, error) {
	_ = ctx
	_ = region
	_ = hypervisorID
	return true, nil
}

func mockNextIP(old string) string {
	parts := strings.Split(old, ".")
	if len(parts) != 4 {
		return "10.66.0.50"
	}
	var last int
	fmt.Sscanf(parts[3], "%d", &last)
	last = (last % 200) + 10
	return fmt.Sprintf("%s.%s.%s.%d", parts[0], parts[1], parts[2], last)
}

func (m *MockAdapter) EnsureServerOS(ctx context.Context, serverID, catalogOSTemplateID, rootPassword string, sshKeys []string) (bool, error) {
	_ = ctx
	_ = serverID
	_ = catalogOSTemplateID
	_ = rootPassword
	_ = sshKeys
	return true, nil
}

func (m *MockAdapter) ReinstallServer(ctx context.Context, id, planID, osTemplateID, rootPassword string, sshKeys []string) error {
	_ = ctx
	_ = rootPassword
	_ = sshKeys
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.servers[id]; ok {
		s.osID = osTemplateID
		s.status = "running"
	}
	return nil
}

func (m *MockAdapter) ResetRootPassword(ctx context.Context, id, osTemplateID string) (string, error) {
	_ = ctx
	_ = id
	_ = osTemplateID
	return "mock-root-pass", nil
}

func (m *MockAdapter) SetRootPassword(ctx context.Context, id, password string) error {
	_ = ctx
	_ = id
	_ = password
	return nil
}

func (m *MockAdapter) GetConsole(ctx context.Context, id string) (*ConsoleSession, error) {
	_ = ctx
	token := randomID()
	return &ConsoleSession{
		Type:     "vnc",
		URL:      fmt.Sprintf("wss://console.vps.local/vnc/%s", id),
		Password: token[:8],
		Token:    token,
	}, nil
}

func (m *MockAdapter) GetMetrics(ctx context.Context, id string) (*Metrics, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.servers[id]; ok {
		metrics := s.metrics
		metrics.UpdatedAt = time.Now().UTC()
		return &metrics, nil
	}
	return &Metrics{
		CPUUsagePercent:  10,
		RAMUsagePercent:  25,
		DiskUsagePercent: 20,
		UpdatedAt:        time.Now().UTC(),
	}, nil
}

func (m *MockAdapter) CreateSnapshot(ctx context.Context, id, name string) (*Snapshot, error) {
	_ = ctx
	return &Snapshot{ID: randomID(), Name: name, Status: "ready"}, nil
}

func (m *MockAdapter) DeleteSnapshot(ctx context.Context, id, snapshotID string) error {
	_ = ctx
	_ = id
	_ = snapshotID
	return nil
}

func (m *MockAdapter) setStatus(ctx context.Context, id, status string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.servers[id]; ok {
		s.status = status
	}
	return nil
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomIP() string {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return fmt.Sprintf("10.66.%d.%d", int(b[0])%200+10, int(b[0])%250+1)
}

func MockIPFromInstance(instanceID string) string {
	if len(instanceID) < 4 {
		return "10.66.0.10"
	}
	sum := 0
	for _, c := range instanceID {
		sum += int(c)
	}
	return net.IPv4(10, 66, byte(sum%200+10), byte(sum%250+1)).String()
}

func (m *MockAdapter) FetchComputeSnapshots(ctx context.Context) ([]ComputeSnapshot, error) {
	_ = ctx
	return []ComputeSnapshot{
		{
			ExternalID:        "1",
			Name:              "NL-1",
			IP:                "66.248.206.14",
			Enabled:           true,
			Maintenance:       false,
			Commissioned:      3,
			MaxServers:        30,
			MaxCPU:            8,
			MaxMemoryMB:       16384,
			CPUAllocated:      2,
			CPUUsedPercent:    25,
			MemoryAllocatedMB: 4096,
			MemoryUsedPercent: 25,
			DiskMaxGB:         500,
			DiskAllocatedGB:   50,
			DiskUsedPercent:   10,
			ServerCount:       1,
		},
	}, nil
}

func (m *MockAdapter) FetchOSImages(ctx context.Context) ([]OSImageRow, error) {
	_ = ctx
	cfg := ActiveOSTemplateMap()
	out := make([]OSImageRow, 0, len(cfg))
	for catalogID, versionID := range cfg {
		t, ok := mockOSTemplateMeta(catalogID)
		if !ok {
			continue
		}
		out = append(out, OSImageRow{Name: t.name, Version: t.version, VersionID: versionID})
	}
	if len(out) == 0 {
		out = []OSImageRow{
			{Name: "Ubuntu", Version: "22.04", VersionID: 1},
			{Name: "Debian", Version: "12", VersionID: 2},
		}
	}
	return out, nil
}

type mockOSMeta struct {
	name    string
	version string
}

func mockOSTemplateMeta(catalogID string) (mockOSMeta, bool) {
	switch catalogID {
	case "ubuntu-22.04":
		return mockOSMeta{"Ubuntu", "22.04"}, true
	case "ubuntu-24.04":
		return mockOSMeta{"Ubuntu", "24.04"}, true
	case "debian-12":
		return mockOSMeta{"Debian", "12"}, true
	default:
		return mockOSMeta{}, false
	}
}
