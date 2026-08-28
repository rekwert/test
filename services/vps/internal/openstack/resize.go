package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

func (a *Adapter) resizeByPlan(ctx context.Context, serverID, catalogPlanID string) error {
	flavorRef, err := a.flavorForPlan(catalogPlanID)
	if err != nil {
		return err
	}
	cli, err := a.clients(ctx)
	if err != nil {
		return err
	}
	flavorID, err := cli.resolveFlavorRef(ctx, flavorRef)
	if err != nil {
		return err
	}
	srv, err := serverByID(cli.Compute, serverID)
	if err != nil {
		return fmt.Errorf("openstack: get server for resize: %w", err)
	}
	if currentFlavorID(srv) == flavorID {
		return nil
	}
	if err := servers.Resize(cli.Compute, serverID, servers.ResizeOpts{
		FlavorRef: flavorID,
	}).ExtractErr(); err != nil {
		if isResizePendingError(err) {
			return nil
		}
		return fmt.Errorf("openstack: resize: %w", err)
	}
	if err := waitServerStatus(cli, serverID, "VERIFY_RESIZE", 600); err != nil {
		// Some clouds auto-confirm and skip VERIFY_RESIZE.
		if err2 := waitServerStatus(cli, serverID, "ACTIVE", 600); err2 != nil {
			return fmt.Errorf("openstack: wait resize: %w", err)
		}
		return nil
	}
	if err := servers.ConfirmResize(cli.Compute, serverID).ExtractErr(); err != nil {
		return fmt.Errorf("openstack: confirm resize: %w", err)
	}
	return waitServerActive(cli, serverID)
}

func currentFlavorID(srv *servers.Server) string {
	if srv == nil || srv.Flavor == nil {
		return ""
	}
	switch v := srv.Flavor["id"].(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func isResizePendingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already") && strings.Contains(msg, "resize") ||
		strings.Contains(msg, "verify_resize") ||
		strings.Contains(msg, "resize_in_progress")
}

func waitServerStatus(cli *Clients, serverID, status string, secs int) error {
	return servers.WaitForStatus(cli.Compute, serverID, status, secs)
}
