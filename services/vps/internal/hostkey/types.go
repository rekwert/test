package hostkey

import "context"

// Client is the Hostkey Invapi surface used by sync/provision/cabinet.
type Client interface {
	ListPresets(ctx context.Context) ([]Preset, error)
	GetPreset(ctx context.Context, presetID int, location string) (*PresetOffer, error)
	ListStocks(ctx context.Context, location string) ([]StockServer, error)
	ListOS(ctx context.Context, presetID int) ([]OSImage, error)
	OrderInstance(ctx context.Context, req OrderRequest) (*OrderResult, error)
	GetServer(ctx context.Context, serverID string) (*Server, error)
	Reboot(ctx context.Context, serverID string) error
	PowerOn(ctx context.Context, serverID string) error
	PowerOff(ctx context.Context, serverID string) error
	Reinstall(ctx context.Context, req ReinstallRequest) (*OrderResult, error)
}

type Preset struct {
	ID          int
	Name        string
	Description string
	CPU         int
	RAMGB       int
	HDD         string
	GPU         string
	Locations   []string
	ServerType  string
	Virtual     bool
	MonthlyEUR  float64
	MonthlyRUB  float64
	Available   int
	PriceByLoc  map[string]LocationPrice
	Tags        map[string]string
}

type LocationPrice struct {
	EUR float64
	RUB float64
	USD float64
}

// PresetOffer is a sellable preset at a specific location.
type PresetOffer struct {
	Preset
	Location string
	PriceEUR float64
	PriceRUB float64
}

type StockServer struct {
	ID          int
	Name        string
	Location    string
	Description string
	CPU         string
	RAMGB       int
	DiskGB      int
	PriceEUR    float64
	PriceRUB    float64
	Status      string
}

type OSImage struct {
	ID   int
	Name string
}

type OrderRequest struct {
	PresetID     int
	StockID      int
	Location     string
	OSID         int
	RootPassword string
	Hostname     string
	DeployPeriod string
	ExtraIPv4    int
	SSHKey       string
}

type ReinstallRequest struct {
	ServerID     string
	Location     string
	OSID         int
	RootPassword string
	Hostname     string
	SSHKey       string
}

type OrderResult struct {
	ServerID     int
	Callback     string
	DeployStatus string
	Status       string
}

type Server struct {
	ID       int
	Hostname string
	Status   string
	Location string
	IPs      []string
	IP       string
}
