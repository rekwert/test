package openstack

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/schedulerhints"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/startstop"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/suspendresume"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

// Adapter implements hypervisor.Adapter against Nova/Neutron/Glance.
type Adapter struct {
	cfg Config
	mu  sync.Mutex
	cli *Clients
}

func NewAdapter(cfg Config) *Adapter {
	return &Adapter{cfg: cfg}
}

func (a *Adapter) clients(ctx context.Context) (*Clients, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cli != nil {
		return a.cli, nil
	}
	cli, err := NewClients(ctx, a.cfg)
	if err != nil {
		return nil, err
	}
	a.cli = cli
	return cli, nil
}

func (a *Adapter) AllocateServer(ctx context.Context, opts hypervisor.CreateOptions) (*hypervisor.Server, error) {
	srv, err := a.createNovaServer(ctx, opts)
	if err != nil {
		return nil, err
	}
	out := mapServer(srv)
	cli, err := a.clients(ctx)
	if err != nil {
		return out, nil
	}
	out.IP = a.primaryIPv4(cli, srv.ID, out.IP)
	return out, nil
}

func (a *Adapter) BuildServer(ctx context.Context, serverID string, opts hypervisor.CreateOptions) error {
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	if err := waitServerActive(cli, serverID); err != nil {
		return err
	}
	if a.cfg.FloatingNetworkID != "" {
		fips, err := a.listServerFloatingIPs(cli, serverID)
		if err != nil {
			return err
		}
		if len(fips) == 0 {
			if ip, err := a.assignFloatingIP(ctx, serverID); err != nil {
				return fmt.Errorf("openstack: assign floating ip: %w", err)
			} else if ip != "" {
				log.Printf("openstack: build assigned floating ip %s to %s", ip, serverID)
			}
		}
	}
	_ = opts
	return nil
}

func (a *Adapter) CreateServer(ctx context.Context, opts hypervisor.CreateOptions) (*hypervisor.Server, error) {
	srv, err := a.createNovaServer(ctx, opts)
	if err != nil {
		return nil, err
	}
	out := mapServer(srv)
	if out.IP == "" {
		out.Status = "building"
	}
	return out, nil
}

func (a *Adapter) createNovaServer(ctx context.Context, opts hypervisor.CreateOptions) (*servers.Server, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, err
	}
	flavorRef, err := a.flavorForPlan(opts.PlanID)
	if err != nil {
		return nil, err
	}
	flavorID, err := cli.resolveFlavorRef(ctx, flavorRef)
	if err != nil {
		return nil, err
	}
	imageRef, err := a.imageForOS(opts.OSTemplateID)
	if err != nil {
		return nil, err
	}
	imageID, err := cli.resolveImageRef(ctx, imageRef)
	if err != nil {
		return nil, err
	}
	networkID, err := cli.resolveNetworkID(ctx, a.cfg)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(opts.Hostname)
	if name == "" {
		name = "vps-" + time.Now().UTC().Format("20060102150405")
	}
	createOpts := servers.CreateOpts{
		Name:      name,
		FlavorRef: flavorID,
		ImageRef:  imageID,
		Networks: []servers.Network{
			{UUID: networkID},
		},
	}
	if opts.RootPassword != "" {
		createOpts.AdminPass = opts.RootPassword
	}
	if ud := buildCloudInitUserData(opts.RootPassword, opts.SSHKeys); len(ud) > 0 {
		createOpts.UserData = ud
	}
	if a.cfg.DefaultAvailabilityZone != "" {
		createOpts.AvailabilityZone = a.cfg.DefaultAvailabilityZone
	}

	createBuilder := servers.CreateOptsBuilder(createOpts)
	if host := a.resolveComputeHost(opts); host != "" {
		createBuilder = schedulerhints.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			SchedulerHints: schedulerhints.SchedulerHints{
				AdditionalProperties: map[string]interface{}{"host": host},
			},
		}
		log.Printf("openstack: scheduling server on host %s", host)
	}

	srv, err := servers.Create(cli.Compute, createBuilder).Extract()
	if err != nil {
		return nil, fmt.Errorf("openstack: create server: %w", err)
	}
	log.Printf("openstack: created server id=%s name=%s", srv.ID, srv.Name)

	if a.cfg.FloatingNetworkID != "" {
		if ip, err := a.assignFloatingIP(ctx, srv.ID); err != nil {
			log.Printf("openstack: floating ip assign for %s: %v", srv.ID, err)
		} else if ip != "" {
			log.Printf("openstack: assigned floating ip %s to %s", ip, srv.ID)
		}
	}
	return srv, nil
}

func (a *Adapter) GetServer(ctx context.Context, id string) (*hypervisor.Server, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, err
	}
	srv, err := serverByID(cli.Compute, id)
	if err != nil {
		return nil, fmt.Errorf("openstack: get server: %w", err)
	}
	out := mapServer(srv)
	out.IP = a.primaryIPv4(cli, id, out.IP)
	return out, nil
}

func (a *Adapter) StartServer(ctx context.Context, id string) error {
	return a.serverAction(ctx, id, func(cli *Clients, id string) error {
		return startstop.Start(cli.Compute, id).ExtractErr()
	})
}

func (a *Adapter) StopServer(ctx context.Context, id string) error {
	return a.serverAction(ctx, id, func(cli *Clients, id string) error {
		return startstop.Stop(cli.Compute, id).ExtractErr()
	})
}

func (a *Adapter) PowerOffServer(ctx context.Context, id string) error {
	return a.serverAction(ctx, id, func(cli *Clients, id string) error {
		return startstop.Stop(cli.Compute, id).ExtractErr()
	})
}

func (a *Adapter) SuspendServer(ctx context.Context, id string) error {
	return a.serverAction(ctx, id, func(cli *Clients, id string) error {
		return suspendresume.Suspend(cli.Compute, id).ExtractErr()
	})
}

func (a *Adapter) UnsuspendServer(ctx context.Context, id string) error {
	return a.serverAction(ctx, id, func(cli *Clients, id string) error {
		return suspendresume.Resume(cli.Compute, id).ExtractErr()
	})
}

func (a *Adapter) RebootServer(ctx context.Context, id string) error {
	return a.serverAction(ctx, id, func(cli *Clients, id string) error {
		return servers.Reboot(cli.Compute, id, servers.RebootOpts{Type: servers.SoftReboot}).ExtractErr()
	})
}

func (a *Adapter) DeleteServer(ctx context.Context, id string) error {
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	if err := a.releaseServerFloatingIPs(cli, id); err != nil {
		log.Printf("openstack: release floating ips for %s: %v", id, err)
	}
	return servers.Delete(cli.Compute, id).ExtractErr()
}

func (a *Adapter) ResizeServer(ctx context.Context, id string, packageID int, preserveDisk bool) error {
	_ = packageID
	_ = preserveDisk
	if packageID <= 0 {
		return nil
	}
	return fmt.Errorf("openstack: ResizeServer requires catalog plan via SyncServerPlan")
}

func (a *Adapter) SyncServerPlan(ctx context.Context, serverID, catalogPlanID string) error {
	return a.resizeByPlan(ctx, serverID, catalogPlanID)
}

func (a *Adapter) AddPrimaryIPv4(ctx context.Context, serverID string) (string, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return "", err
	}
	existing, err := a.listServerFloatingIPs(cli, serverID)
	if err != nil {
		return "", err
	}
	if len(existing) == 0 {
		portID, err := a.primaryPortID(cli, serverID)
		if err != nil {
			return "", err
		}
		ip, err := a.createFloatingIPForPort(cli, portID)
		if err != nil {
			return "", err
		}
		if ip == "" {
			return "", fmt.Errorf("openstack: no floating ip assigned")
		}
		return ip, nil
	}
	return a.swapPrimaryFloatingIP(ctx, serverID)
}

func (a *Adapter) AddExtraIPv4(ctx context.Context, serverID string, qty int) ([]string, error) {
	if qty <= 0 {
		return nil, fmt.Errorf("openstack: invalid ipv4 quantity")
	}
	out := make([]string, 0, qty)
	for i := 0; i < qty; i++ {
		ip, err := a.attachExtraInterfaceWithFIP(ctx, serverID)
		if err != nil {
			if len(out) > 0 {
				return out, err
			}
			return nil, err
		}
		out = append(out, ip)
	}
	return out, nil
}

func (a *Adapter) RemovePrimaryIPv4(ctx context.Context, serverID, ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("openstack: ip required")
	}
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	fip, err := a.findFloatingIPByAddress(cli, ip)
	if err != nil {
		return err
	}
	if err := disassociateFloatingIP(cli, fip.ID); err != nil {
		return fmt.Errorf("openstack: disassociate %s: %w", ip, err)
	}
	if err := deleteFloatingIP(cli, fip.ID); err != nil {
		return fmt.Errorf("openstack: delete floating ip %s: %w", ip, err)
	}
	_ = serverID
	return nil
}

func (a *Adapter) SyncServerNetworkFilters(_ context.Context, _ string) error {
	return nil
}

func (a *Adapter) PrimaryIPv4Info(ctx context.Context, serverID string) ([]string, string, string, error) {
	return a.serverPublicIPs(ctx, serverID)
}

func (a *Adapter) HasFreePrimaryIPv4(ctx context.Context, _, _ string) (bool, error) {
	if a.cfg.FloatingNetworkID == "" {
		return true, nil
	}
	return a.hasFreeFloatingIP(ctx)
}

func (a *Adapter) EnsureServerOS(ctx context.Context, serverID, catalogOSTemplateID, rootPassword string, sshKeys []string) (bool, error) {
	_ = catalogOSTemplateID
	_ = rootPassword
	_ = sshKeys
	srv, err := a.GetServer(ctx, serverID)
	if err != nil {
		return false, err
	}
	if strings.EqualFold(srv.Status, "ACTIVE") || strings.EqualFold(srv.Status, "running") {
		return true, nil
	}
	if strings.EqualFold(srv.Status, "ERROR") {
		return false, fmt.Errorf("openstack: server %s in ERROR", serverID)
	}
	return false, nil
}

func (a *Adapter) ReinstallServer(ctx context.Context, id, planID, osTemplateID, rootPassword string, sshKeys []string) error {
	_ = planID
	_ = sshKeys
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	imageRef, err := a.imageForOS(osTemplateID)
	if err != nil {
		return err
	}
	imageID, err := cli.resolveImageRef(ctx, imageRef)
	if err != nil {
		return err
	}
	opts := servers.RebuildOpts{ImageRef: imageID}
	if rootPassword != "" {
		opts.AdminPass = rootPassword
	}
	_, err = servers.Rebuild(cli.Compute, id, opts).Extract()
	if err != nil {
		return fmt.Errorf("openstack: rebuild: %w", err)
	}
	return waitServerActive(cli, id)
}

func (a *Adapter) ResetRootPassword(ctx context.Context, id, _ string) (string, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return "", err
	}
	pass := randomPassword(16)
	if err := servers.ChangeAdminPassword(cli.Compute, id, pass).ExtractErr(); err != nil {
		return "", fmt.Errorf("openstack: change admin password: %w", err)
	}
	return pass, nil
}

func (a *Adapter) SetRootPassword(ctx context.Context, id, password string) error {
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	if err := servers.ChangeAdminPassword(cli.Compute, id, password).ExtractErr(); err != nil {
		return fmt.Errorf("openstack: set root password: %w", err)
	}
	return nil
}

func (a *Adapter) serverAction(ctx context.Context, id string, fn func(*Clients, string) error) error {
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	return fn(cli, id)
}

func (a *Adapter) flavorForPlan(planID string) (string, error) {
	ref, ok := a.cfg.FlavorRef(planID)
	if !ok {
		return "", fmt.Errorf("openstack: no flavor mapping for plan %q", planID)
	}
	return ref, nil
}

func (a *Adapter) imageForOS(catalogOS string) (string, error) {
	ref, ok := a.cfg.ImageRef(catalogOS)
	if !ok {
		return "", fmt.Errorf("openstack: no image mapping for os %q", catalogOS)
	}
	return ref, nil
}

func (a *Adapter) resolveComputeHost(opts hypervisor.CreateOptions) string {
	if host := strings.TrimSpace(opts.ComputeHost); host != "" {
		return host
	}
	return a.cfg.ComputeHost(opts.NodeID, opts.HypervisorExternalID)
}

func (a *Adapter) primaryIPv4(cli *Clients, serverID, fallback string) string {
	if fallback != "" {
		return fallback
	}
	fips, err := a.listServerFloatingIPs(cli, serverID)
	if err == nil && len(fips) > 0 {
		return fips[0].Address
	}
	page, err := ports.List(cli.Network, ports.ListOpts{DeviceID: serverID}).AllPages()
	if err != nil {
		return fallback
	}
	all, err := ports.ExtractPorts(page)
	if err != nil {
		return fallback
	}
	for _, p := range all {
		for _, ip := range p.FixedIPs {
			if ip.IPAddress != "" {
				return ip.IPAddress
			}
		}
	}
	return fallback
}

func mapServer(srv *servers.Server) *hypervisor.Server {
	status := strings.ToLower(strings.TrimSpace(srv.Status))
	ip := strings.TrimSpace(srv.AccessIPv4)
	if ip == "" {
		ip = extractIPv4FromAddresses(srv.Addresses)
	}
	osBuilt := status == "active" || status == "running"
	return &hypervisor.Server{
		ID:               srv.ID,
		Status:           status,
		RealStatus:       status,
		IP:               ip,
		OSBuilt:          osBuilt,
		GuestAgentActive: osBuilt,
		RemoteStateKnown: true,
		RemoteState:      osBuilt,
	}
}

func extractIPv4FromAddresses(addrs map[string]interface{}) string {
	for _, raw := range addrs {
		items, ok := raw.([]interface{})
		if !ok {
			continue
		}
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			version, _ := m["version"].(float64)
			addr, _ := m["addr"].(string)
			if int(version) == 4 && addr != "" {
				return addr
			}
		}
	}
	return ""
}
