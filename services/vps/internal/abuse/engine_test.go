package abuse

import (
	"testing"
	"time"
)

func baseCfg() Config {
	return Config{
		AutoStopThreshold:     100,
		NewInstanceGraceBonus: 40,
		MinDistinctSignals:    2,
		Weights:               copyWeights(DefaultWeights),
	}
}

func rec(signalType string, weight int) SignalRecord {
	return SignalRecord{Type: signalType, Weight: weight, CreatedAt: time.Now()}
}

func TestEvaluate_CriticalProviderComplaint(t *testing.T) {
	cfg := baseCfg()
	signals := []SignalRecord{rec(SignalProviderComplaint, 100)}
	d := Evaluate(cfg, signals, 7*24*time.Hour)
	if !d.AutoStop || d.Reason != "critical:provider_complaint" {
		t.Fatalf("expected critical auto-stop, got %+v", d)
	}
}

func TestEvaluate_SingleSMTPNoStop(t *testing.T) {
	cfg := baseCfg()
	signals := []SignalRecord{rec(SignalSMTPPortOpen, 35)}
	d := Evaluate(cfg, signals, 7*24*time.Hour)
	if d.AutoStop {
		t.Fatalf("single smtp should not auto-stop: %+v", d)
	}
}

func TestEvaluate_SMTPPlusTxFloodCombo(t *testing.T) {
	cfg := baseCfg()
	signals := []SignalRecord{
		rec(SignalSMTPPortOpen, 50),
		rec(SignalSustainedTxFlood, 45),
	}
	d := Evaluate(cfg, signals, 7*24*time.Hour)
	if !d.AutoStop || d.Reason != "combo:smtp_tx_flood" {
		t.Fatalf("expected combo stop, got %+v", d)
	}
}

func TestEvaluate_MultiSignalThreshold(t *testing.T) {
	cfg := baseCfg()
	signals := []SignalRecord{
		rec(SignalExternalReport, 75),
		rec(SignalPortScanOutbound, 60),
	}
	d := Evaluate(cfg, signals, 7*24*time.Hour)
	if !d.AutoStop || d.Reason != "multi_signal" {
		t.Fatalf("expected multi_signal stop, got %+v", d)
	}
}

func TestEvaluate_NewInstanceGrace(t *testing.T) {
	cfg := baseCfg()
	signals := []SignalRecord{
		rec(SignalExternalReport, 75),
		rec(SignalPortScanOutbound, 60),
	}
	d := Evaluate(cfg, signals, 6*time.Hour)
	if d.AutoStop {
		t.Fatalf("young instance with score 135 vs threshold 140 should not stop: %+v", d)
	}
}

func TestEvaluate_OneSignalBelowThreshold(t *testing.T) {
	cfg := baseCfg()
	signals := []SignalRecord{rec(SignalExternalReport, 75)}
	d := Evaluate(cfg, signals, 30*24*time.Hour)
	if d.AutoStop {
		t.Fatalf("single non-critical signal must not stop: %+v", d)
	}
}

func TestUpdateTxStreak_RequiresConsecutivePolls(t *testing.T) {
	cfg := baseCfg()
	cfg.TxMbpsThreshold = 100
	cfg.TxSustainedPolls = 4

	st := DetectorState{}
	var sig *Signal
	for i := 0; i < 3; i++ {
		st, sig = UpdateTxStreak(cfg, st, 200)
	}
	if sig != nil {
		t.Fatal("should not signal before sustained polls")
	}
	st, sig = UpdateTxStreak(cfg, st, 200)
	if sig == nil || sig.Type != SignalSustainedTxFlood {
		t.Fatalf("expected flood signal on 4th poll, got %+v", sig)
	}
}

func TestUpdateTxStreak_ResetsOnLowTraffic(t *testing.T) {
	cfg := baseCfg()
	cfg.TxMbpsThreshold = 100
	cfg.TxSustainedPolls = 4

	st := DetectorState{}
	st, _ = UpdateTxStreak(cfg, st, 200)
	st, _ = UpdateTxStreak(cfg, st, 200)
	st, _ = UpdateTxStreak(cfg, st, 10)
	if st.TxHighStreak != 0 {
		t.Fatalf("expected streak reset, got %d", st.TxHighStreak)
	}
}

func TestSMTPSignal_Port25Escalates(t *testing.T) {
	cfg := baseCfg()
	sig := SMTPSignal(cfg, []int{25})
	if sig == nil || sig.Weight < 50 {
		t.Fatalf("port 25 should escalate weight: %+v", sig)
	}
}

func TestSMTPSignal_NoPortsNil(t *testing.T) {
	cfg := baseCfg()
	if SMTPSignal(cfg, nil) != nil {
		t.Fatal("expected nil")
	}
}
