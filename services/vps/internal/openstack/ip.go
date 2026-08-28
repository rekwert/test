package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/attachinterfaces"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/pagination"
)

type floatingIPRecord struct {
	ID      string
	Address string
	PortID  string
}

func (a *Adapter) primaryPortID(cli *Clients, serverID string) (string, error) {
	return a.serverPortID(cli, serverID)
}

func (a *Adapter) listServerFloatingIPs(cli *Clients, serverID string) ([]floatingIPRecord, error) {
	portIDs := map[string]struct{}{}
	page, err := ports.List(cli.Network, ports.ListOpts{DeviceID: serverID}).AllPages()
	if err != nil {
		return nil, err
	}
	allPorts, err := ports.ExtractPorts(page)
	if err != nil {
		return nil, err
	}
	for _, p := range allPorts {
		if p.ID != "" {
			portIDs[p.ID] = struct{}{}
		}
	}
	if len(portIDs) == 0 {
		return nil, nil
	}

	var out []floatingIPRecord
	fipPage, err := floatingips.List(cli.Network, floatingips.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}
	allFIPs, err := floatingips.ExtractFloatingIPs(fipPage)
	if err != nil {
		return nil, err
	}
	for _, fip := range allFIPs {
		if _, ok := portIDs[fip.PortID]; !ok || strings.TrimSpace(fip.FloatingIP) == "" {
			continue
		}
		out = append(out, floatingIPRecord{
			ID:      fip.ID,
			Address: strings.TrimSpace(fip.FloatingIP),
			PortID:  fip.PortID,
		})
	}
	return out, nil
}

func (a *Adapter) findFloatingIPByAddress(cli *Clients, address string) (*floatingIPRecord, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("openstack: empty floating ip address")
	}
	page, err := floatingips.List(cli.Network, floatingips.ListOpts{
		FloatingIP: address,
	}).AllPages()
	if err != nil {
		return nil, err
	}
	all, err := floatingips.ExtractFloatingIPs(page)
	if err != nil {
		return nil, err
	}
	for _, fip := range all {
		if strings.TrimSpace(fip.FloatingIP) == address {
			return &floatingIPRecord{
				ID:      fip.ID,
				Address: address,
				PortID:  fip.PortID,
			}, nil
		}
	}
	return nil, fmt.Errorf("openstack: floating ip %q not found", address)
}

func disassociateFloatingIP(cli *Clients, fipID string) error {
	empty := ""
	_, err := floatingips.Update(cli.Network, fipID, floatingips.UpdateOpts{
		PortID: &empty,
	}).Extract()
	return err
}

func deleteFloatingIP(cli *Clients, fipID string) error {
	return floatingips.Delete(cli.Network, fipID).ExtractErr()
}

func (a *Adapter) releaseServerFloatingIPs(cli *Clients, serverID string) error {
	fips, err := a.listServerFloatingIPs(cli, serverID)
	if err != nil {
		return err
	}
	for _, fip := range fips {
		if err := disassociateFloatingIP(cli, fip.ID); err != nil {
			return fmt.Errorf("disassociate %s: %w", fip.Address, err)
		}
		if err := deleteFloatingIP(cli, fip.ID); err != nil {
			return fmt.Errorf("delete floating ip %s: %w", fip.Address, err)
		}
	}
	return nil
}

func (a *Adapter) createFloatingIPForPort(cli *Clients, portID string) (string, error) {
	if a.cfg.FloatingNetworkID == "" {
		return "", fmt.Errorf("openstack: OPENSTACK_FLOATING_NETWORK_ID is required for public ipv4")
	}
	fip, err := floatingips.Create(cli.Network, floatingips.CreateOpts{
		FloatingNetworkID: a.cfg.FloatingNetworkID,
		PortID:            portID,
	}).Extract()
	if err != nil {
		return "", fmt.Errorf("create floating ip: %w", err)
	}
	if fip == nil || strings.TrimSpace(fip.FloatingIP) == "" {
		return "", fmt.Errorf("openstack: floating ip missing after create")
	}
	return strings.TrimSpace(fip.FloatingIP), nil
}

func (a *Adapter) swapPrimaryFloatingIP(ctx context.Context, serverID string) (string, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return "", err
	}
	portID, err := a.primaryPortID(cli, serverID)
	if err != nil {
		return "", err
	}
	existing, err := a.listServerFloatingIPs(cli, serverID)
	if err != nil {
		return "", err
	}
	for _, fip := range existing {
		if fip.PortID == portID {
			if err := disassociateFloatingIP(cli, fip.ID); err != nil {
				return "", fmt.Errorf("disassociate floating ip %s: %w", fip.Address, err)
			}
		}
	}
	newIP, err := a.createFloatingIPForPort(cli, portID)
	if err != nil {
		return "", err
	}
	return newIP, nil
}

func (a *Adapter) attachExtraInterfaceWithFIP(ctx context.Context, serverID string) (string, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return "", err
	}
	networkID, err := cli.resolveNetworkID(ctx, a.cfg)
	if err != nil {
		return "", err
	}
	iface, err := attachinterfaces.Create(cli.Compute, serverID, attachinterfaces.CreateOpts{
		NetworkID: networkID,
	}).Extract()
	if err != nil {
		return "", fmt.Errorf("attach interface: %w", err)
	}
	if iface == nil || strings.TrimSpace(iface.PortID) == "" {
		return "", fmt.Errorf("openstack: missing port after interface attach")
	}
	return a.createFloatingIPForPort(cli, iface.PortID)
}

func (a *Adapter) serverPublicIPs(ctx context.Context, serverID string) ([]string, string, string, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, "", "", err
	}
	var ips []string
	if a.cfg.FloatingNetworkID != "" {
		fips, err := a.listServerFloatingIPs(cli, serverID)
		if err != nil {
			return nil, "", "", err
		}
		for _, fip := range fips {
			ips = append(ips, fip.Address)
		}
	}
	if len(ips) == 0 {
		page, err := ports.List(cli.Network, ports.ListOpts{DeviceID: serverID}).AllPages()
		if err != nil {
			return nil, "", "", err
		}
		allPorts, err := ports.ExtractPorts(page)
		if err != nil {
			return nil, "", "", err
		}
		for _, p := range allPorts {
			for _, fixed := range p.FixedIPs {
				if fixed.IPAddress != "" {
					ips = append(ips, fixed.IPAddress)
				}
			}
		}
	}
	if len(ips) == 0 {
		srv, err := serverByID(cli.Compute, serverID)
		if err == nil {
			if ip := strings.TrimSpace(srv.AccessIPv4); ip != "" {
				ips = append(ips, ip)
			}
			if ip := extractIPv4FromAddresses(srv.Addresses); ip != "" {
				ips = append(ips, ip)
			}
		}
	}
	if len(ips) == 0 {
		return nil, "", "", fmt.Errorf("openstack: server has no ipv4")
	}
	gateway, netmask := a.subnetGatewayNetmask(cli, serverID)
	return dedupeStrings(ips), gateway, netmask, nil
}

func (a *Adapter) subnetGatewayNetmask(cli *Clients, serverID string) (string, string) {
	page, err := ports.List(cli.Network, ports.ListOpts{DeviceID: serverID}).AllPages()
	if err != nil {
		return "", "255.255.255.0"
	}
	allPorts, err := ports.ExtractPorts(page)
	if err != nil || len(allPorts) == 0 {
		return "", "255.255.255.0"
	}
	for _, p := range allPorts {
		for _, fixed := range p.FixedIPs {
			if fixed.SubnetID == "" {
				continue
			}
			subnet, err := subnets.Get(cli.Network, fixed.SubnetID).Extract()
			if err != nil || subnet == nil {
				continue
			}
			prefix := cidrPrefix(subnet.CIDR)
			if prefix == 0 {
				prefix = 24
			}
			return strings.TrimSpace(subnet.GatewayIP), prefixToNetmask(prefix)
		}
	}
	return "", "255.255.255.0"
}

func (a *Adapter) hasFreeFloatingIP(ctx context.Context) (bool, error) {
	if a.cfg.FloatingNetworkID == "" {
		return false, fmt.Errorf("openstack: floating network not configured")
	}
	cli, err := a.clients(ctx)
	if err != nil {
		return false, err
	}
	fip, err := floatingips.Create(cli.Network, floatingips.CreateOpts{
		FloatingNetworkID: a.cfg.FloatingNetworkID,
	}).Extract()
	if err != nil {
		if isFloatingPoolExhausted(err) {
			return false, nil
		}
		return false, err
	}
	if fip == nil || fip.ID == "" {
		return false, fmt.Errorf("openstack: floating ip probe failed")
	}
	_ = deleteFloatingIP(cli, fip.ID)
	return true, nil
}

func isFloatingPoolExhausted(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not enough") ||
		strings.Contains(msg, "no more") ||
		strings.Contains(msg, "unable to allocate") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "409")
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func cidrPrefix(cidr string) int {
	cidr = strings.TrimSpace(cidr)
	if i := strings.LastIndex(cidr, "/"); i >= 0 && i+1 < len(cidr) {
		var p int
		fmt.Sscanf(cidr[i+1:], "%d", &p)
		if p > 0 && p <= 32 {
			return p
		}
	}
	return 0
}

func prefixToNetmask(prefix int) string {
	if prefix <= 0 || prefix > 32 {
		return "255.255.255.0"
	}
	mask := uint32(0xFFFFFFFF) << (32 - prefix)
	return fmt.Sprintf("%d.%d.%d.%d",
		(mask>>24)&0xFF, (mask>>16)&0xFF, (mask>>8)&0xFF, mask&0xFF)
}

// listFloatingIPsByPort is used in tests and diagnostics.
func listFloatingIPsByPort(cli *Clients, portID string) ([]floatingIPRecord, error) {
	var out []floatingIPRecord
	err := floatingips.List(cli.Network, floatingips.ListOpts{PortID: portID}).EachPage(func(page pagination.Page) (bool, error) {
		items, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, err
		}
		for _, fip := range items {
			out = append(out, floatingIPRecord{
				ID:      fip.ID,
				Address: strings.TrimSpace(fip.FloatingIP),
				PortID:  fip.PortID,
			})
		}
		return true, nil
	})
	return out, err
}
