package abuse

import (
	"encoding/json"
	"time"
)

// DetectorState persists streak counters between worker polls (stored in instances.abuse_state).
type DetectorState struct {
	TxHighStreak   int       `json:"tx_high_streak"`
	LastTxMbps     float64   `json:"last_tx_mbps,omitempty"`
	LastSMTPProbe  time.Time `json:"last_smtp_probe,omitempty"`
	SMTPOpenPorts  []int     `json:"smtp_open_ports,omitempty"`
	LastSignalAt   map[string]time.Time `json:"last_signal_at,omitempty"`
}

func ParseDetectorState(raw json.RawMessage) DetectorState {
	if len(raw) == 0 {
		return DetectorState{LastSignalAt: map[string]time.Time{}}
	}
	var st DetectorState
	if err := json.Unmarshal(raw, &st); err != nil {
		return DetectorState{LastSignalAt: map[string]time.Time{}}
	}
	if st.LastSignalAt == nil {
		st.LastSignalAt = map[string]time.Time{}
	}
	return st
}

func (st DetectorState) Marshal() json.RawMessage {
	b, _ := json.Marshal(st)
	return b
}

// UpdateTxStreak returns updated state and optional signal when sustained outbound flood detected.
func UpdateTxStreak(cfg Config, st DetectorState, txMbps float64) (DetectorState, *Signal) {
	st.LastTxMbps = txMbps
	if txMbps >= cfg.TxMbpsThreshold {
		st.TxHighStreak++
	} else {
		st.TxHighStreak = 0
		return st, nil
	}
	if st.TxHighStreak < cfg.TxSustainedPolls {
		return st, nil
	}
	// Fire once per streak peak; reset streak after signal to require sustained pattern again.
	st.TxHighStreak = 0
	weight := cfg.Weight(SignalSustainedTxFlood)
	// Escalate weight when extremely high throughput.
	if txMbps >= cfg.TxMbpsThreshold*3 {
		weight += 20
	}
	return st, &Signal{
		Type:   SignalSustainedTxFlood,
		Weight: weight,
		Evidence: map[string]any{
			"tx_mbps":           txMbps,
			"threshold_mbps":    cfg.TxMbpsThreshold,
			"sustained_polls":   cfg.TxSustainedPolls,
		},
	}
}

func ShouldProbeSMTP(cfg Config, st DetectorState, now time.Time) bool {
	if st.LastSMTPProbe.IsZero() {
		return true
	}
	return now.Sub(st.LastSMTPProbe) >= cfg.SMTPProbeInterval
}

func SMTPSignal(cfg Config, openPorts []int) *Signal {
	if len(openPorts) == 0 {
		return nil
	}
	// Port 25 open on a generic VPS is a strong spam indicator; 465/587 alone may be legitimate apps.
	weight := cfg.Weight(SignalSMTPPortOpen)
	for _, p := range openPorts {
		if p == 25 {
			weight += 15
			break
		}
	}
	return &Signal{
		Type:   SignalSMTPPortOpen,
		Weight: weight,
		Evidence: map[string]any{
			"open_ports": openPorts,
		},
	}
}
