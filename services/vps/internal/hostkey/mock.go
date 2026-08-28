package hostkey

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type MockClient struct {
	mu      sync.Mutex
	presets []Preset
	servers map[string]*Server
	seq     atomic.Int64
}

func NewMock() *MockClient {
	return &MockClient{
		servers: map[string]*Server{},
		presets: []Preset{
			{
				ID: 801001, Name: "bm.mock-1", Description: "Mock Xeon · 32GB · 2x960GB SSD",
				CPU: 4, RAMGB: 32, HDD: "2x960", Locations: []string{"NL", "DE"},
				ServerType: "Instant Dedicated Server", Virtual: false,
				MonthlyEUR: 50, MonthlyRUB: 4500, Available: 2,
				PriceByLoc: map[string]LocationPrice{
					"NL": {EUR: 50, RUB: 4500},
					"DE": {EUR: 70, RUB: 7000},
				},
				Tags: map[string]string{"bm": "true", "cpu_info": "4 Cores 3.2GHz"},
			},
		},
	}
}

func (m *MockClient) ListPresets(ctx context.Context) ([]Preset, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Preset, len(m.presets))
	copy(out, m.presets)
	return out, nil
}

func (m *MockClient) GetPreset(ctx context.Context, presetID int, location string) (*PresetOffer, error) {
	presets, err := m.ListPresets(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range presets {
		if p.ID == presetID {
			offer := presetOfferAtLocation(p, location)
			if offer == nil {
				return nil, fmt.Errorf("hostkey mock: preset %d not in %s", presetID, location)
			}
			return offer, nil
		}
	}
	return nil, fmt.Errorf("hostkey mock: preset %d not found", presetID)
}

func (m *MockClient) ListStocks(ctx context.Context, location string) ([]StockServer, error) {
	_ = ctx
	_ = location
	return nil, nil
}

func (m *MockClient) ListOS(ctx context.Context, presetID int) ([]OSImage, error) {
	_ = ctx
	_ = presetID
	return []OSImage{
		{ID: 219, Name: "Debian 12"},
		{ID: 237, Name: "Ubuntu 24.04"},
	}, nil
}

func (m *MockClient) OrderInstance(ctx context.Context, req OrderRequest) (*OrderResult, error) {
	_ = ctx
	id := int(90000 + m.seq.Add(1))
	ip := fmt.Sprintf("203.0.113.%d", 100+id%100)
	m.mu.Lock()
	m.servers[fmt.Sprintf("%d", id)] = &Server{
		ID: id, Hostname: req.Hostname, Status: "rent", Location: req.Location, IP: ip, IPs: []string{ip},
	}
	m.mu.Unlock()
	return &OrderResult{ServerID: id, DeployStatus: "install", Status: "OK"}, nil
}

func (m *MockClient) GetServer(ctx context.Context, serverID string) (*Server, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverID]
	if !ok {
		return nil, fmt.Errorf("hostkey mock: server %s not found", serverID)
	}
	cp := *s
	return &cp, nil
}

func (m *MockClient) Reboot(ctx context.Context, serverID string) error {
	_, err := m.GetServer(ctx, serverID)
	return err
}

func (m *MockClient) PowerOn(ctx context.Context, serverID string) error {
	_, err := m.GetServer(ctx, serverID)
	return err
}

func (m *MockClient) PowerOff(ctx context.Context, serverID string) error {
	_, err := m.GetServer(ctx, serverID)
	return err
}

func (m *MockClient) Reinstall(ctx context.Context, req ReinstallRequest) (*OrderResult, error) {
	id, err := strconv.Atoi(strings.TrimSpace(req.ServerID))
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("hostkey mock: invalid server id")
	}
	if _, err := m.GetServer(ctx, req.ServerID); err != nil {
		return nil, err
	}
	return &OrderResult{ServerID: id, DeployStatus: "reinstall"}, nil
}
