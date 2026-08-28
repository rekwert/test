package store

import (
	"context"
	"errors"
)

// resolveOrderNode decides immediate placement vs capacity waitlist for a new order.
// Region tier toggles (region_tiers) control whether a line is sold in a GEO.
// When sold but no online node hosts the tier, orders queue until hardware is linked.
// Midrange and hustle always queue when hardware exists; promotion picks dedicated nodes.
func (s *Store) resolveOrderNode(ctx context.Context, region, tier string) (nodeID string, waitlisted bool, err error) {
	enabled, err := s.RegionTierEnabled(ctx, region, tier)
	if err != nil {
		return "", false, err
	}
	if !enabled {
		return "", false, ErrNoNodeForRegion
	}

	hasNode, checkErr := s.RegionHasOnlineNode(ctx, region, tier)
	if checkErr != nil {
		return "", false, checkErr
	}
	if !hasNode {
		return "", true, nil
	}

	if TierAcceptsCapacityWaitlist(tier) {
		return "", true, nil
	}

	nodeID, err = s.PickNodeForRegion(ctx, region, tier)
	if err == nil {
		return nodeID, false, nil
	}
	if !errors.Is(err, ErrNoNodeForRegion) {
		return "", false, err
	}
	return "", true, nil
}
