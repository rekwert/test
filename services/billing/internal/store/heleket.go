package store

import (
	"context"
	"fmt"
	"strings"
)

func compactUUIDToStandard(compact string) string {
	compact = strings.ReplaceAll(compact, "-", "")
	if len(compact) != 32 {
		return compact
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		compact[0:8], compact[8:12], compact[12:16], compact[16:20], compact[20:32])
}

// MarkInvoicePaidByHeleketRef credits balance using Heleket order_id and/or payment uuid.
func (s *Store) MarkInvoicePaidByHeleketRef(ctx context.Context, orderID, heleketUUID string, webhookAmount float64) (bool, error) {
	orderID = strings.TrimSpace(orderID)
	heleketUUID = strings.TrimSpace(heleketUUID)

	if orderID != "" {
		credited, err := s.MarkInvoicePaidByID(ctx, orderID, webhookAmount)
		if err != nil || credited {
			return credited, err
		}
		if !strings.Contains(orderID, "-") {
			credited, err = s.MarkInvoicePaidByID(ctx, compactUUIDToStandard(orderID), webhookAmount)
			if err != nil || credited {
				return credited, err
			}
		}
	}
	if heleketUUID != "" {
		return s.MarkInvoicePaid(ctx, heleketUUID, webhookAmount)
	}
	return false, nil
}
