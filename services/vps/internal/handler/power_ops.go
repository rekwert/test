package handler

import (
	"context"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func (h *Handler) stopInstancePower(ctx context.Context, instanceID, externalID string) error {
	if externalID == "" {
		return nil
	}
	provider, productType, _, err := h.store.GetInstanceProvider(ctx, instanceID)
	if err != nil {
		return err
	}
	isDedicated := store.IsDedicatedProvider(provider) || productType == "dedicated"
	if isDedicated {
		if provider == "hostkey" {
			hk := h.hostkey
			if hk == nil {
				hk = hostkey.NewFromEnv()
			}
			return hk.PowerOff(ctx, externalID)
		}
		robot := h.robot
		if robot == nil {
			robot = hetznerrobot.NewFromEnv()
		}
		return robot.Reset(ctx, externalID, "power")
	}
	return runPowerWithRetry(ctx, func() error {
		return h.hv().StopServer(ctx, externalID)
	})
}
