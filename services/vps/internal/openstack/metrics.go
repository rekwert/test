package openstack

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/diagnostics"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

func (a *Adapter) GetMetrics(ctx context.Context, id string) (*hypervisor.Metrics, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, err
	}
	out := &hypervisor.Metrics{UpdatedAt: time.Now().UTC()}

	diags, err := diagnostics.Get(cli.Compute, id).Extract()
	if err != nil {
		return out, nil
	}
	applyDiagnosticsMetrics(out, diags)
	return out, nil
}

func applyDiagnosticsMetrics(out *hypervisor.Metrics, diags map[string]interface{}) {
	if out == nil || len(diags) == 0 {
		return
	}
	if v, ok := diagFloat(diags, "cpu"); ok {
		out.CPUUsagePercent = clampPercent(v)
	} else if v, ok := diagFloat(diags, "cpu0"); ok {
		out.CPUUsagePercent = clampPercent(v)
	}

	memActual, hasActual := diagFloat(diags, "memory-actual")
	mem, hasMem := diagFloat(diags, "memory")
	switch {
	case hasActual && hasMem && mem > 0:
		out.RAMUsagePercent = clampPercent(memActual / mem * 100)
	case hasMem:
		out.RAMUsagePercent = clampPercent(mem / (1024 * 1024) * 100)
	}

	if disk, ok := diagFloat(diags, "vda_errors"); ok && disk > 0 {
		out.DiskUsagePercent = clampPercent(disk)
	}

	rx, rxOK := sumDiagPrefix(diags, "_rx")
	tx, txOK := sumDiagPrefix(diags, "_tx")
	if rxOK {
		out.RxMbps = rx / 1_000_000
	}
	if txOK {
		out.TxMbps = tx / 1_000_000
	}
}

func diagFloat(diags map[string]interface{}, key string) (float64, bool) {
	raw, ok := diags[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func sumDiagPrefix(diags map[string]interface{}, suffix string) (float64, bool) {
	var sum float64
	var found bool
	for key, raw := range diags {
		if !strings.HasSuffix(key, suffix) {
			continue
		}
		switch v := raw.(type) {
		case float64:
			sum += v
			found = true
		case int:
			sum += float64(v)
			found = true
		case int64:
			sum += float64(v)
			found = true
		}
	}
	return sum, found
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
