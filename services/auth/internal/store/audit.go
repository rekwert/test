package store

import (
	"context"
	"encoding/json"
	"time"
)

type AuditEntry struct {
	ID        int64          `json:"id"`
	ActorID   *string        `json:"actor_id,omitempty"`
	Action    string         `json:"action"`
	Entity    string         `json:"entity"`
	EntityID  *string        `json:"entity_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	IP        *string        `json:"ip,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type AuditListFilters struct {
	UserID string
	Limit  int
	Offset int
}

func (s *Store) InsertAuditLog(ctx context.Context, actorID, action, entity, entityID string, metadata map[string]any, conn ConnectionMeta) error {
	var metaJSON []byte
	if metadata != nil {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	var actorArg, entityIDArg, ipArg any
	if actorID != "" {
		actorArg = actorID
	}
	if entityID != "" {
		entityIDArg = entityID
	}
	if conn.IP != "" {
		ipArg = conn.IP
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth.audit_log (actor_id, action, entity, entity_id, metadata, ip, client_port, server_port)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6::inet, $7, $8)
	`, actorArg, action, entity, entityIDArg, metaJSON, ipArg, conn.ClientPort, conn.ServerPort)
	return err
}

func (s *Store) ListAuditLog(ctx context.Context, f AuditListFilters) ([]AuditEntry, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}

	userFilter := f.UserID
	likeUser := ""
	if userFilter != "" {
		likeUser = userFilter
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, actor_id::text, action, entity, entity_id, metadata, host(ip)::text, created_at
		FROM auth.audit_log
		WHERE ($1 = '' OR entity_id = $1 OR actor_id::text = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, likeUser, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		var actorID, entityID, ip *string
		var meta []byte
		if err := rows.Scan(&e.ID, &actorID, &e.Action, &e.Entity, &entityID, &meta, &ip, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		e.ActorID = actorID
		e.EntityID = entityID
		e.IP = ip
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &e.Metadata)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM auth.audit_log
		WHERE ($1 = '' OR entity_id = $1 OR actor_id::text = $1)
	`, likeUser).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
