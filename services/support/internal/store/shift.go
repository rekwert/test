package store

import (
	"context"
	"fmt"
	"time"
)

type StaffShift struct {
	StaffID   string
	OnDuty    bool
	StartedAt *time.Time
}

type SlotConfig struct {
	MaxNewSlots    int
	MaxReturnSlots int
	RebindHours    int
	AutoCloseHours int
}

type SlotUsage struct {
	NewInProgress    int
	ReturnInProgress int
	ReturnPending    int
}

func (s *Store) GetSlotConfig(ctx context.Context) (SlotConfig, error) {
	var cfg SlotConfig
	err := s.pool.QueryRow(ctx, `
		SELECT max_new_slots, max_return_slots, rebind_hours, auto_close_hours
		FROM support.slot_config WHERE id = 1
	`).Scan(&cfg.MaxNewSlots, &cfg.MaxReturnSlots, &cfg.RebindHours, &cfg.AutoCloseHours)
	if err != nil {
		return SlotConfig{MaxNewSlots: 2, MaxReturnSlots: 2, RebindHours: 2, AutoCloseHours: 12}, nil
	}
	return cfg, nil
}

func (s *Store) UpdateSlotConfig(ctx context.Context, cfg SlotConfig) (SlotConfig, error) {
	if cfg.MaxNewSlots < 1 || cfg.MaxNewSlots > 20 {
		return SlotConfig{}, fmt.Errorf("max_new_slots out of range")
	}
	if cfg.MaxReturnSlots < 0 || cfg.MaxReturnSlots > 20 {
		return SlotConfig{}, fmt.Errorf("max_return_slots out of range")
	}
	if cfg.RebindHours < 1 || cfg.RebindHours > 72 {
		return SlotConfig{}, fmt.Errorf("rebind_hours out of range")
	}
	if cfg.AutoCloseHours < 1 || cfg.AutoCloseHours > 168 {
		return SlotConfig{}, fmt.Errorf("auto_close_hours out of range")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE support.slot_config
		SET max_new_slots = $1,
		    max_return_slots = $2,
		    rebind_hours = $3,
		    auto_close_hours = $4
		WHERE id = 1
	`, cfg.MaxNewSlots, cfg.MaxReturnSlots, cfg.RebindHours, cfg.AutoCloseHours)
	if err != nil {
		return SlotConfig{}, err
	}
	return s.GetSlotConfig(ctx)
}

func (s *Store) GetShift(ctx context.Context, staffID string) (StaffShift, error) {
	var sh StaffShift
	sh.StaffID = staffID
	err := s.pool.QueryRow(ctx, `
		SELECT on_duty, started_at FROM support.staff_shifts WHERE staff_id = $1
	`, staffID).Scan(&sh.OnDuty, &sh.StartedAt)
	if err != nil {
		return StaffShift{StaffID: staffID, OnDuty: false}, nil
	}
	return sh, nil
}

func (s *Store) SetShiftOnDuty(ctx context.Context, staffID string, onDuty bool) (StaffShift, error) {
	now := time.Now().UTC()
	var started *time.Time
	if onDuty {
		started = &now
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO support.staff_shifts (staff_id, on_duty, started_at, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (staff_id) DO UPDATE
		SET on_duty = $2, started_at = $3, updated_at = now()
	`, staffID, onDuty, started)
	if err != nil {
		return StaffShift{}, err
	}
	return s.GetShift(ctx, staffID)
}

func (s *Store) CountSlots(ctx context.Context, assigneeID string) (SlotUsage, error) {
	var u SlotUsage
	err := s.pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE status = 'in_progress' AND NOT is_return),
		  COUNT(*) FILTER (WHERE status = 'in_progress' AND is_return),
		  COUNT(*) FILTER (WHERE status = 'return_pending')
		FROM support.tickets
		WHERE assignee_id = $1
	`, assigneeID).Scan(&u.NewInProgress, &u.ReturnInProgress, &u.ReturnPending)
	return u, err
}
