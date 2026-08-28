package hetznerrobot

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// MockClient serves local/dev without Robot credentials.
type MockClient struct {
	mu       sync.Mutex
	products []MarketProduct
	servers  map[string]*Server
	txs      map[string]*Transaction
	seq      atomic.Int64
}

func NewMock() *MockClient {
	m := &MockClient{
		servers: map[string]*Server{},
		txs:     map[string]*Transaction{},
		products: []MarketProduct{
			{
				ID: 900001, Name: "SB-MOCK-1", CPU: "AMD EPYC 7502P", CPUBenchmark: 20000,
				MemoryGB: 64, DiskGB: 512, DiskCount: 2, DiskText: "NVMe",
				Datacenter: "FSN1-DC1", NetworkSpeed: "1 Gbit/s", Traffic: "20 TB",
				PriceEUR: 39.0, Dist: []string{"Ubuntu 24.04.1 LTS base", "Debian 12.5 base"},
				Addons: []string{"primary_ipv4"}, Source: "market",
				Description: []string{"AMD EPYC 7502P", "64 GB RAM", "2x 512 GB NVMe"},
			},
			{
				ID: 900002, Name: "SB-MOCK-2", CPU: "Intel Core i7-8700", CPUBenchmark: 12000,
				MemoryGB: 32, DiskGB: 1000, DiskCount: 2, DiskText: "SSD",
				Datacenter: "NBG1-DC3", NetworkSpeed: "1 Gbit/s", Traffic: "20 TB",
				PriceEUR: 29.5, Dist: []string{"Ubuntu 22.04.4 LTS base", "Debian 12.5 base"},
				Addons: []string{"primary_ipv4"}, Source: "market",
				Description: []string{"Intel Core i7-8700", "32 GB RAM", "2x 1 TB SSD"},
			},
		},
	}
	return m
}

func (m *MockClient) ListMarketProducts(ctx context.Context) ([]MarketProduct, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MarketProduct, len(m.products))
	copy(out, m.products)
	return out, nil
}

func (m *MockClient) GetMarketProduct(ctx context.Context, productID int) (*MarketProduct, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.products {
		if m.products[i].ID == productID {
			p := m.products[i]
			return &p, nil
		}
	}
	return nil, fmt.Errorf("hetzner robot: market product %d not found", productID)
}

func (m *MockClient) ListServerProducts(ctx context.Context) ([]ServerProduct, error) {
	_ = ctx
	return []ServerProduct{
		{
			ID: "AX41", Name: "AX41", CPU: "AMD Ryzen 5 3600", MemoryGB: 64, DiskGB: 512,
			Datacenter: "FSN1", PriceEUR: 44, Dist: []string{"Ubuntu 24.04.1 LTS base"},
			Location: []string{"FSN1"}, Description: []string{"AX41 dedicated"},
		},
	}, nil
}

func (m *MockClient) OrderMarket(ctx context.Context, productID int, addons []string, password, authorizedKey string) (*Transaction, error) {
	_ = ctx
	_ = addons
	_ = password
	_ = authorizedKey
	p, err := m.GetMarketProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("MOCK-%d", m.seq.Add(1))
	num := int(10000 + m.seq.Load())
	ip := fmt.Sprintf("203.0.113.%d", 10+m.seq.Load()%200)
	tx := &Transaction{
		ID:           id,
		Status:       "completed",
		ServerNumber: &num,
		ServerIP:     &ip,
		ProductID:    p.ID,
		ProductName:  p.Name,
	}
	m.mu.Lock()
	m.txs[id] = tx
	m.servers[fmt.Sprintf("%d", num)] = &Server{
		Number: num, Name: p.Name, IP: ip, IPs: []string{ip}, Product: p.Name, DC: p.Datacenter, Status: "ready",
	}
	m.mu.Unlock()
	return tx, nil
}

func (m *MockClient) OrderServerAddon(ctx context.Context, serverNumber int, productID, reason string) (*Transaction, error) {
	_ = productID
	_ = reason
	key := fmt.Sprintf("%d", serverNumber)
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[key]
	if !ok {
		return nil, fmt.Errorf("hetzner robot: server %d not found", serverNumber)
	}
	n := len(s.IPs) + 1
	extra := fmt.Sprintf("203.0.113.%d", 50+n)
	s.IPs = append(s.IPs, extra)
	id := fmt.Sprintf("MOCK-ADDON-%d", m.seq.Add(1))
	tx := &Transaction{ID: id, Status: "ready", ServerNumber: &serverNumber, ServerIP: &extra}
	m.txs[id] = tx
	return tx, nil
}

func (m *MockClient) GetTransaction(ctx context.Context, id string) (*Transaction, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, ok := m.txs[id]
	if !ok {
		return nil, fmt.Errorf("hetzner robot: transaction %s not found", id)
	}
	cp := *tx
	return &cp, nil
}

func (m *MockClient) GetAddonTransaction(ctx context.Context, id string) (*Transaction, error) {
	return m.GetTransaction(ctx, id)
}

func (m *MockClient) GetServer(ctx context.Context, serverNumber string) (*Server, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverNumber]
	if !ok {
		return nil, fmt.Errorf("hetzner robot: server %s not found", serverNumber)
	}
	cp := *s
	return &cp, nil
}

func (m *MockClient) Reset(ctx context.Context, serverNumber, resetType string) error {
	_ = ctx
	_ = resetType
	_, err := m.GetServer(ctx, serverNumber)
	return err
}

func (m *MockClient) EnableRescue(ctx context.Context, serverNumber, os string) (string, error) {
	_ = os
	if _, err := m.GetServer(ctx, serverNumber); err != nil {
		return "", err
	}
	return "mock-rescue-pass", nil
}

func (m *MockClient) ActivateLinux(ctx context.Context, serverNumber, dist, lang string) (string, error) {
	_ = dist
	_ = lang
	if _, err := m.GetServer(ctx, serverNumber); err != nil {
		return "", err
	}
	return "mock-root-pass", nil
}

func (m *MockClient) ListLinuxDist(ctx context.Context, serverNumber string) ([]string, error) {
	if _, err := m.GetServer(ctx, serverNumber); err != nil {
		return nil, err
	}
	return []string{"Ubuntu 24.04.1 LTS base", "Debian 12.5 base"}, nil
}
