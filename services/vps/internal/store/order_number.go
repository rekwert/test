package store

import "context"

func (s *Store) GetInstanceOrderNumber(ctx context.Context, instanceID string) (int64, error) {
	var n *int64
	err := s.pool.QueryRow(ctx, `
		SELECT o.order_number
		FROM vps.instances i
		LEFT JOIN vps.orders o ON o.id = i.order_id
		WHERE i.id = $1::uuid
	`, instanceID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if n == nil {
		return 0, nil
	}
	return *n, nil
}
