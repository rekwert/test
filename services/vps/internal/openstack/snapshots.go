package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

func (a *Adapter) CreateSnapshot(ctx context.Context, id, name string) (*hypervisor.Snapshot, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" {
		return nil, fmt.Errorf("openstack: missing server id for snapshot")
	}
	if name == "" {
		name = "snapshot"
	}
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, err
	}
	imageID, err := servers.CreateImage(cli.Compute, id, servers.CreateImageOpts{
		Name: name,
	}).ExtractImageID()
	if err != nil {
		return nil, fmt.Errorf("openstack: create snapshot: %w", err)
	}
	status := "creating"
	if img, err := images.Get(cli.Image, imageID).Extract(); err == nil && img != nil {
		status = strings.ToLower(strings.TrimSpace(string(img.Status)))
		if status == "" {
			status = "creating"
		}
	}
	return &hypervisor.Snapshot{
		ID:     imageID,
		Name:   name,
		Status: status,
	}, nil
}

func (a *Adapter) DeleteSnapshot(ctx context.Context, _, snapshotID string) error {
	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return fmt.Errorf("openstack: missing snapshot id")
	}
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	if err := images.Delete(cli.Image, snapshotID).ExtractErr(); err != nil {
		return fmt.Errorf("openstack: delete snapshot: %w", err)
	}
	return nil
}
