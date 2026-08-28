package store

import (
	"context"
	"encoding/json"
)

func (s *Store) InsertAuthAudit(ctx context.Context, actorID, action, entity, entityID string, metadata map[string]any) error {
	var metaJSON []byte
	if metadata != nil {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	var actorArg, entityIDArg any
	if actorID != "" {
		actorArg = actorID
	}
	if entityID != "" {
		entityIDArg = entityID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth.audit_log (actor_id, action, entity, entity_id, metadata, ip)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, NULL)
	`, actorArg, action, entity, entityIDArg, metaJSON)
	return err
}
