package hetznerrobot

import "context"

// Client is the Hetzner Robot surface used by sync/provision/cabinet.
type Client interface {
	ListMarketProducts(ctx context.Context) ([]MarketProduct, error)
	GetMarketProduct(ctx context.Context, productID int) (*MarketProduct, error)
	ListServerProducts(ctx context.Context) ([]ServerProduct, error)
	OrderMarket(ctx context.Context, productID int, addons []string, password, authorizedKey string) (*Transaction, error)
	GetTransaction(ctx context.Context, id string) (*Transaction, error)
	OrderServerAddon(ctx context.Context, serverNumber int, productID, reason string) (*Transaction, error)
	GetAddonTransaction(ctx context.Context, id string) (*Transaction, error)
	GetServer(ctx context.Context, serverNumber string) (*Server, error)
	Reset(ctx context.Context, serverNumber, resetType string) error
	EnableRescue(ctx context.Context, serverNumber, os string) (password string, err error)
	ActivateLinux(ctx context.Context, serverNumber, dist, lang string) (password string, err error)
	ListLinuxDist(ctx context.Context, serverNumber string) ([]string, error)
}

type MarketProduct struct {
	ID            int
	Name          string
	Description   []string
	CPU           string
	CPUBenchmark  int
	MemoryGB      int
	DiskGB        int
	DiskCount     int
	DiskText      string
	Datacenter    string
	NetworkSpeed  string
	Traffic       string
	PriceEUR      float64
	PriceSetupEUR float64
	FixedPrice    bool
	Dist          []string
	Addons        []string
	Source        string // market | product
}

type ServerProduct struct {
	ID            string
	Name          string
	Description   []string
	CPU           string
	MemoryGB      int
	DiskGB        int
	Datacenter    string
	PriceEUR      float64
	PriceSetupEUR float64
	Dist          []string
	Location      []string
}

type Transaction struct {
	ID           string
	Status       string
	ServerNumber *int
	ServerIP     *string
	ProductID    int
	ProductName  string
}

type Server struct {
	Number    int
	Name      string
	IP        string
	IPs       []string // all IPv4 on the server (primary + additional)
	IPv6Net   string
	Product   string
	DC        string
	Status    string
	Cancelled bool
}
