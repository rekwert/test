package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SMTPControlTarget is enough to apply HV smtp allow/deny for a VPS.
type SMTPControlTarget struct {
	InstanceID       string
	UserID           string
	IP               string
	Provider         string
	ProductType      string
	HVHost           string
	SmtpOutboundOpen bool
}

func (s *Store) GetSMTPControlTarget(ctx context.Context, instanceID string) (*SMTPControlTarget, error) {
	var t SMTPControlTarget
	var ip *string
	var hv *string
	err := s.pool.QueryRow(ctx, `
		SELECT i.id::text,
		       i.user_id::text,
		       host(i.ip_address)::text,
		       COALESCE(i.provider, 'openstack'),
		       COALESCE(i.product_type, 'vps'),
		       NULLIF(TRIM(n.vf_ip), ''),
		       COALESCE(i.smtp_outbound_open, false)
		FROM vps.instances i
		LEFT JOIN vps.nodes n ON n.id = i.node_id
		WHERE i.id = $1::uuid
		  AND i.state <> 'deleted'
	`, instanceID).Scan(
		&t.InstanceID, &t.UserID, &ip, &t.Provider, &t.ProductType, &hv, &t.SmtpOutboundOpen,
	)
	if err != nil {
		return nil, err
	}
	if ip != nil {
		t.IP = *ip
	}
	if hv != nil {
		t.HVHost = *hv
	}
	return &t, nil
}

func (s *Store) SetSmtpOutboundOpen(ctx context.Context, instanceID string, open bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET smtp_outbound_open = $2,
		    updated_at = now()
		WHERE id = $1::uuid
		  AND state <> 'deleted'
	`, instanceID, open)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
