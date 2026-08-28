package abuse

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Signal types — weights define severity; critical types auto-stop alone.
const (
	SignalProviderComplaint = "provider_complaint"
	SignalBlacklistHit      = "blacklist_hit"
	SignalExternalReport      = "external_report"
	SignalPortScanOutbound    = "port_scan_outbound"
	SignalSustainedTxFlood    = "sustained_tx_flood"
	SignalSMTPPortOpen        = "smtp_port_open"
)

var DefaultWeights = map[string]int{
	SignalProviderComplaint: 100,
	SignalBlacklistHit:      95,
	SignalExternalReport:    75,
	SignalPortScanOutbound:  60,
	SignalSustainedTxFlood:  45,
	SignalSMTPPortOpen:      35,
}

type Signal struct {
	Type     string
	Weight   int
	Evidence map[string]any
}

type SignalRecord struct {
	Type      string
	Weight    int
	CreatedAt time.Time
}

type Config struct {
	Enabled              bool
	AutoStopThreshold    int
	NewInstanceGraceBonus int
	MinDistinctSignals   int
	SignalWindow         time.Duration
	SignalDedupe         time.Duration
	TxMbpsThreshold      float64
	TxSustainedPolls     int
	SMTPProbeInterval    time.Duration
	SMTPPorts            []int
	Weights              map[string]int
}

func LoadConfig() Config {
	cfg := Config{
		Enabled:               envBool("ABUSE_ENABLED", true),
		AutoStopThreshold:     envInt("ABUSE_AUTO_STOP_THRESHOLD", 100),
		NewInstanceGraceBonus: envInt("ABUSE_NEW_INSTANCE_GRACE_BONUS", 40),
		MinDistinctSignals:    envInt("ABUSE_MIN_DISTINCT_SIGNALS", 2),
		SignalWindow:          envDuration("ABUSE_SIGNAL_WINDOW", time.Hour),
		SignalDedupe:          envDuration("ABUSE_SIGNAL_DEDUPE", 15*time.Minute),
		TxMbpsThreshold:       envFloat("ABUSE_TX_MBPS_THRESHOLD", 150),
		TxSustainedPolls:      envInt("ABUSE_TX_SUSTAINED_POLLS", 4),
		SMTPProbeInterval:     envDuration("ABUSE_SMTP_PROBE_INTERVAL", 10*time.Minute),
		SMTPPorts:             []int{25, 465, 587},
		Weights:               copyWeights(DefaultWeights),
	}
	if raw := strings.TrimSpace(os.Getenv("ABUSE_SMTP_PORTS")); raw != "" {
		parts := strings.Split(raw, ",")
		ports := make([]int, 0, len(parts))
		for _, p := range parts {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 {
				ports = append(ports, n)
			}
		}
		if len(ports) > 0 {
			cfg.SMTPPorts = ports
		}
	}
	return cfg
}

func (c Config) Weight(signalType string) int {
	if w, ok := c.Weights[signalType]; ok {
		return w
	}
	return 30
}

type Decision struct {
	AutoStop       bool
	Reason         string
	TotalScore     int
	DistinctTypes  int
	EffectiveThreshold int
}

// Evaluate decides whether accumulated signals warrant an automatic stop.
// Rules:
//  1. Any critical signal (weight >= AutoStopThreshold) → immediate stop.
//  2. Otherwise require MinDistinctSignals different types AND total score >= threshold.
//  3. SMTP + sustained TX flood combo (both present) lowers bar to 80 score with 2 types.
//  4. Young instances get a grace bonus added to threshold (harder to auto-stop).
func Evaluate(cfg Config, signals []SignalRecord, instanceAge time.Duration) Decision {
	threshold := cfg.AutoStopThreshold
	if instanceAge > 0 && instanceAge < 48*time.Hour {
		threshold += cfg.NewInstanceGraceBonus
	}

	byType := aggregateByType(signals)
	total := 0
	types := make([]string, 0, len(byType))
	for t, w := range byType {
		types = append(types, t)
		total += w
	}

	dec := Decision{
		TotalScore:         total,
		DistinctTypes:      len(types),
		EffectiveThreshold: threshold,
	}

	for t, w := range byType {
		if w >= cfg.AutoStopThreshold || isCriticalType(t) && w >= 90 {
			dec.AutoStop = true
			dec.Reason = "critical:" + t
			return dec
		}
	}

	hasSMTP := byType[SignalSMTPPortOpen] > 0
	hasTx := byType[SignalSustainedTxFlood] > 0
	if hasSMTP && hasTx && total >= 80 && len(types) >= 2 {
		dec.AutoStop = true
		dec.Reason = "combo:smtp_tx_flood"
		return dec
	}

	if len(types) >= cfg.MinDistinctSignals && total >= threshold {
		dec.AutoStop = true
		dec.Reason = "multi_signal"
		return dec
	}

	// Single sustained flood is serious only after many consecutive detections recorded as one type
	// with elevated weight — handled via repeated Insert with streak; here allow single-type stop
	// only if score clearly exceeds threshold + grace.
	if byType[SignalSustainedTxFlood] >= threshold && byType[SignalSustainedTxFlood] >= 90 {
		dec.AutoStop = true
		dec.Reason = "sustained_tx_flood"
		return dec
	}

	return dec
}

func aggregateByType(signals []SignalRecord) map[string]int {
	out := make(map[string]int)
	for _, s := range signals {
		if s.Weight > out[s.Type] {
			out[s.Type] = s.Weight
		}
	}
	return out
}

func isCriticalType(t string) bool {
	switch t {
	case SignalProviderComplaint, SignalBlacklistHit:
		return true
	default:
		return false
	}
}

func copyWeights(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
