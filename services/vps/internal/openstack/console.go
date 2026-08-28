package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/remoteconsoles"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

func (a *Adapter) GetConsole(ctx context.Context, id string) (*hypervisor.ConsoleSession, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("openstack: missing server id for console")
	}
	console, err := remoteconsoles.Create(cli.Compute, id, remoteconsoles.CreateOpts{
		Protocol: remoteconsoles.ConsoleProtocolVNC,
		Type:     remoteconsoles.ConsoleTypeNoVNC,
	}).Extract()
	if err != nil {
		return nil, fmt.Errorf("openstack: create console: %w", err)
	}
	if console == nil || strings.TrimSpace(console.URL) == "" {
		return nil, fmt.Errorf("openstack: console url missing")
	}
	return &hypervisor.ConsoleSession{
		Type: "vnc",
		URL:  console.URL,
	}, nil
}
