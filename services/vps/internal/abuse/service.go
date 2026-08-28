package abuse

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

type Service struct {
	cfg Config
	st  *store.Store
	hv  hypervisor.Adapter
}

func NewService(cfg Config, st *store.Store, hv hypervisor.Adapter) *Service {
	return &Service{cfg: cfg, st: st, hv: hv}
}

func (s *Service) Enabled() bool {
	return s.cfg.Enabled && s.st != nil
}

// RecordSignal ingests a signal, evaluates policy, and may auto-stop the instance.
func (s *Service) RecordSignal(ctx context.Context, instanceID, userID string, sig Signal) (autoStopped bool, err error) {
	if !s.Enabled() {
		return false, nil
	}
	if instanceID == "" || userID == "" || sig.Type == "" {
		return false, fmt.Errorf("invalid signal")
	}
	if sig.Weight <= 0 {
		sig.Weight = s.cfg.Weight(sig.Type)
	}

	dupSince := time.Now().UTC().Add(-s.cfg.SignalDedupe)
	exists, err := s.st.RecentAbuseSignalExists(ctx, instanceID, sig.Type, dupSince)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	evidence, _ := json.Marshal(sig.Evidence)
	if _, err := s.st.InsertAbuseSignal(ctx, instanceID, userID, sig.Type, sig.Weight, evidence); err != nil {
		return false, err
	}

	return s.evaluateAndMaybeStop(ctx, instanceID, userID)
}

func (s *Service) evaluateAndMaybeStop(ctx context.Context, instanceID, userID string) (bool, error) {
	open, err := s.st.HasOpenAbuseCase(ctx, instanceID)
	if err != nil {
		return false, err
	}
	if open {
		return false, nil
	}

	since := time.Now().UTC().Add(-s.cfg.SignalWindow)
	rows, err := s.st.ListRecentAbuseSignals(ctx, instanceID, since)
	if err != nil {
		return false, err
	}
	records := make([]SignalRecord, 0, len(rows))
	for _, r := range rows {
		records = append(records, SignalRecord{
			Type:      r.SignalType,
			Weight:    r.Weight,
			CreatedAt: r.CreatedAt,
		})
	}

	var age time.Duration
	inst, err := s.st.GetInstanceByID(ctx, instanceID)
	if err == nil && inst.CreatedAt != nil {
		age = time.Since(*inst.CreatedAt)
	}

	dec := Evaluate(s.cfg, records, age)
	if !dec.AutoStop {
		return false, nil
	}

	ext, extErr := s.st.GetInstanceExternalID(ctx, instanceID)
	if extErr == nil && ext != "" && s.hv != nil {
		if stopErr := s.hv.StopServer(ctx, ext); stopErr != nil {
			log.Printf("abuse auto-stop hv %s: %v", instanceID, stopErr)
			return false, fmt.Errorf("hypervisor stop failed: %w", stopErr)
		}
	}

	triggerJSON, _ := json.Marshal(records)
	caseID, err := s.st.CreateAbuseCase(ctx, instanceID, userID, "auto_stopped", dec.Reason, dec.TotalScore, triggerJSON)
	if err != nil {
		return false, err
	}

	if err := s.st.ApplyAbuseAutoStop(ctx, instanceID, userID, caseID, dec.Reason, dec.TotalScore, triggerJSON); err != nil {
		return false, err
	}

	hostname := "server"
	ip := ""
	if inst != nil {
		if inst.Hostname != nil {
			hostname = *inst.Hostname
		}
		if inst.IPAddress != nil {
			ip = *inst.IPAddress
		}
	}

	_ = notify.AbuseAutoStopEmail(ctx, userID, hostname, ip, caseID)
	_ = notify.OpsAlert(ctx, "Abuse auto-stop",
		fmt.Sprintf("instance=%s user=%s ip=%s reason=%s score=%d case=%s", instanceID, userID, ip, dec.Reason, dec.TotalScore, caseID))

	return true, nil
}

// ScanInstance runs local detectors (metrics, SMTP ports) for one running instance.
func (s *Service) ScanInstance(ctx context.Context, row store.AbuseScanInstance) error {
	if !s.Enabled() || row.AbuseHold {
		return nil
	}
	if row.IPAddress == nil || *row.IPAddress == "" {
		return nil
	}

	st := ParseDetectorState(row.AbuseState)
	now := time.Now().UTC()
	var metrics hypervisor.Metrics
	if len(row.Metrics) > 0 {
		_ = json.Unmarshal(row.Metrics, &metrics)
		var sig *Signal
		st, sig = UpdateTxStreak(s.cfg, st, metrics.TxMbps)
		if sig != nil {
			if _, err := s.RecordSignal(ctx, row.ID, row.UserID, *sig); err != nil {
				return err
			}
		}
	}

	if ShouldProbeSMTP(s.cfg, st, now) {
		st.LastSMTPProbe = now
		open := ProbeSMTPPortsContext(ctx, *row.IPAddress, s.cfg.SMTPPorts)
		st.SMTPOpenPorts = open
		if sig := SMTPSignal(s.cfg, open); sig != nil {
			if _, err := s.RecordSignal(ctx, row.ID, row.UserID, *sig); err != nil {
				return err
			}
		}
	}

	return s.st.UpdateInstanceAbuseState(ctx, row.ID, st.Marshal())
}

func (s *Service) ScanBatch(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	rows, err := s.st.ListInstancesForAbuseScan(ctx, 40)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.ScanInstance(ctx, row); err != nil {
			log.Printf("abuse scan %s: %v", row.ID, err)
		}
	}
	return nil
}
