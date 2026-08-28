package openstack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
)

func (a *Adapter) assignFloatingIP(ctx context.Context, serverID string) (string, error) {
	if a.cfg.FloatingNetworkID == "" {
		return "", nil
	}
	cli, err := a.clients(ctx)
	if err != nil {
		return "", err
	}
	portID, err := a.serverPortID(cli, serverID)
	if err != nil {
		return "", err
	}
	return a.createFloatingIPForPort(cli, portID)
}

func (a *Adapter) serverPortID(cli *Clients, serverID string) (string, error) {
	page, err := ports.List(cli.Network, ports.ListOpts{DeviceID: serverID}).AllPages()
	if err != nil {
		return "", err
	}
	all, err := ports.ExtractPorts(page)
	if err != nil {
		return "", err
	}
	for _, p := range all {
		if p.ID != "" {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("openstack: no port for server %s", serverID)
}

func waitServerActive(cli *Clients, serverID string) error {
	return servers.WaitForStatus(cli.Compute, serverID, "ACTIVE", 600)
}
