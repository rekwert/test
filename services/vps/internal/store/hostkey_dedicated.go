package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/google/uuid"
)

var hostkeyPlanNS = uuid.MustParse("b2c3d4e5-f6a7-8901-bcde-f12345678901")

func HostkeyPlanID(providerKey string) string {
	return uuid.NewSHA1(hostkeyPlanNS, []byte(providerKey)).String()
}

func (s *Store) UpsertHostkeyPresetPlans(ctx context.Context, cfg hostkey.Config, presets []hostkey.Preset, osNames map[int][]string) (int, error) {
	seen := make([]string, 0)
	n := 0
	for _, p := range presets {
		if p.ID <= 0 || p.Available <= 0 {
			continue
		}
		cpuModel := hostkeyPresetCPUModel(p)
		diskText := p.HDD
		if diskText == "" {
			diskText = tagExtra(p.Tags, "web_storage")
		}
		for _, loc := range hostkey.EffectivePresetLocations(p) {
			offer := hostkey.PresetOffer{Preset: p, Location: loc}
			if lp, ok := p.PriceByLoc[loc]; ok {
				offer.PriceEUR, offer.PriceRUB = lp.EUR, lp.RUB
			} else {
				offer.PriceEUR, offer.PriceRUB = p.MonthlyEUR, p.MonthlyRUB
			}
			if offer.PriceEUR <= 0 && offer.PriceRUB <= 0 {
				continue
			}
			extID := hostkey.FormatExternalProductID(p.ID, loc)
			planID := HostkeyPlanID("preset:" + extID)
			seen = append(seen, planID)
			region := hostkey.RegionFromLocation(loc)
			price := cfg.SellPrice(offer.PriceEUR, offer.PriceRUB)
			if price <= 0 {
				continue
			}
			osList := osNames[p.ID]
			meta, _ := json.Marshal(map[string]any{
				"source":        "preset",
				"preset_id":     p.ID,
				"location_code": loc,
				"cpu_model":     cpuModel,
				"memory_gb":     p.RAMGB,
				"disk_text":     diskText,
				"datacenter":    loc,
				"price_eur":     offer.PriceEUR,
				"price_rub":     offer.PriceRUB,
				"product_name":  p.Name,
				"description":   []string{p.Description},
				"server_type":   p.ServerType,
				"dist":          osList,
				"network_speed": p.Tags["web_port_speed"],
				"traffic":       p.Tags["web_traffic"],
			})
			ramMb := p.RAMGB * 1024
			if ramMb <= 0 {
				ramMb = 4096
			}
			diskGB := 500
			cpu := p.CPU
			if cpu <= 0 {
				cpu = 4
			}
			base := strings.TrimSpace(p.Name)
			if cpuModel != "" {
				base = strings.TrimSpace(cpuModel)
			}
			name := fmt.Sprintf("%s · %s · #%d", base, loc, p.ID)
			if _, err := s.pool.Exec(ctx, `
				INSERT INTO vps.plans (
					id, name, cpu, ram_mb, disk_gb, price_monthly, region, active,
					product_type, provider, external_product_id, provider_meta, available, synced_at
				) VALUES (
					$1::uuid, $2, $3, $4, $5, $6, $7, true,
					'dedicated', 'hostkey', $8, $9::jsonb, true, now()
				)
				ON CONFLICT (id) DO UPDATE SET
					name = EXCLUDED.name,
					cpu = EXCLUDED.cpu,
					ram_mb = EXCLUDED.ram_mb,
					disk_gb = EXCLUDED.disk_gb,
					price_monthly = EXCLUDED.price_monthly,
					region = EXCLUDED.region,
					active = true,
					product_type = 'dedicated',
					provider = 'hostkey',
					external_product_id = EXCLUDED.external_product_id,
					provider_meta = EXCLUDED.provider_meta,
					available = true,
					synced_at = now()
			`, planID, name, cpu, ramMb, diskGB, price, region, extID, meta); err != nil {
				return n, err
			}
			n++
		}
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE vps.plans
		SET available = false, active = false, synced_at = now()
		WHERE product_type = 'dedicated'
		  AND provider = 'hostkey'
		  AND COALESCE(provider_meta->>'source', 'preset') = 'preset'
		  AND NOT (id = ANY($1::uuid[]))
	`, seen); err != nil {
		return n, err
	}
	return n, nil
}

func (s *Store) UpsertHostkeyStockPlans(ctx context.Context, cfg hostkey.Config, stocks []hostkey.StockServer) (int, error) {
	seen := make([]string, 0, len(stocks))
	n := 0
	for _, st := range stocks {
		if st.ID <= 0 {
			continue
		}
		extID := hostkey.FormatStockExternalProductID(st.ID)
		planID := HostkeyPlanID("stock:" + extID)
		seen = append(seen, planID)
		region := hostkey.RegionFromLocation(st.Location)
		price := cfg.SellPrice(st.PriceEUR, st.PriceRUB)
		if price <= 0 {
			continue
		}
		cpuModel := hostkeyStockCPUModel(st)
		meta, _ := json.Marshal(map[string]any{
			"source":        "stock",
			"stock_id":      st.ID,
			"location_code": st.Location,
			"cpu_model":     cpuModel,
			"memory_gb":     st.RAMGB,
			"disk_gb":       st.DiskGB,
			"datacenter":    st.Location,
			"price_eur":     st.PriceEUR,
			"price_rub":     st.PriceRUB,
			"product_name":  st.Name,
			"description":   []string{st.Description},
		})
		ramMb := st.RAMGB * 1024
		if ramMb <= 0 {
			ramMb = 8192
		}
		disk := st.DiskGB
		if disk <= 0 {
			disk = 500
		}
		name := strings.TrimSpace(st.Name)
		if name == "" {
			name = fmt.Sprintf("Stock #%d · %s", st.ID, st.Location)
		} else {
			name = fmt.Sprintf("%s · #%d", name, st.ID)
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO vps.plans (
				id, name, cpu, ram_mb, disk_gb, price_monthly, region, active,
				product_type, provider, external_product_id, provider_meta, available, synced_at
			) VALUES (
				$1::uuid, $2, $3, $4, $5, $6, $7, true,
				'dedicated', 'hostkey', $8, $9::jsonb, true, now()
			)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				cpu = EXCLUDED.cpu,
				ram_mb = EXCLUDED.ram_mb,
				disk_gb = EXCLUDED.disk_gb,
				price_monthly = EXCLUDED.price_monthly,
				region = EXCLUDED.region,
				active = true,
				provider_meta = EXCLUDED.provider_meta,
				available = true,
				synced_at = now()
		`, planID, name, 4, ramMb, disk, price, region, extID, meta); err != nil {
			return n, err
		}
		n++
	}
	if len(seen) == 0 {
		return n, nil
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE vps.plans
		SET available = false, active = false, synced_at = now()
		WHERE product_type = 'dedicated'
		  AND provider = 'hostkey'
		  AND COALESCE(provider_meta->>'source', '') = 'stock'
		  AND NOT (id = ANY($1::uuid[]))
	`, seen); err != nil {
		return n, err
	}
	return n, nil
}

func ValidateHostkeyPrice(catalogPriceRub, liveEUR, liveRUB float64, cfg hostkey.Config) error {
	live := cfg.SellPrice(liveEUR, liveRUB)
	if live <= 0 {
		return ErrLotUnavailable
	}
	limit := catalogPriceRub * (1 + cfg.PriceSlackPct/100)
	if live > limit+0.01 {
		return fmt.Errorf("%w: catalog=%.2f live=%.2f", ErrLotPriceChanged, catalogPriceRub, live)
	}
	return nil
}

func tagExtra(tags map[string]string, key string) string {
	if tags == nil {
		return ""
	}
	return strings.TrimSpace(tags[key])
}

func hostkeyPresetCPUModel(p hostkey.Preset) string {
	if cpu := hostkeyCPUFromBMLine(p.Description); cpu != "" {
		return cpu
	}
	for _, key := range []string{"web_cpu_info", "cpu_info", "web_description", "description"} {
		if cpu := hostkeyCPUFromBMLine(p.Tags[key]); cpu != "" {
			return cpu
		}
	}
	candidates := []string{
		p.Tags["cpu_info"],
		p.Tags["web_cpu_info"],
		strings.TrimSpace(p.Name),
	}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" || hostkeyIsGenericCoreLabel(c) {
			continue
		}
		if hostkeyLooksLikeCPUModel(c) {
			return c
		}
	}
	if cpu := hostkeyCPUFromBMLine(p.Name); cpu != "" {
		return cpu
	}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c != "" && !hostkeyIsGenericCoreLabel(c) {
			return c
		}
	}
	return ""
}

func hostkeyStockCPUModel(st hostkey.StockServer) string {
	if cpu := hostkeyCPUFromBMLine(st.Description); cpu != "" {
		return cpu
	}
	cpu := strings.TrimSpace(st.CPU)
	if cpu != "" && !hostkeyIsGenericCoreLabel(cpu) {
		return cpu
	}
	if cpu := hostkeyCPUFromBMLine(st.Name); cpu != "" {
		return cpu
	}
	return cpu
}

func hostkeyLooksLikeCPUModel(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "xeon") ||
		strings.Contains(lower, "ryzen") ||
		strings.Contains(lower, "epyc") ||
		strings.Contains(lower, "core i") ||
		strings.Contains(lower, "threadripper") ||
		strings.Contains(lower, "intel") ||
		strings.Contains(lower, "amd")
}

func hostkeyPriceEURFromMeta(meta map[string]any) float64 {
	if meta == nil {
		return 0
	}
	if v, ok := meta["price_eur"].(float64); ok {
		return v
	}
	return 0
}

func hostkeyLocationFromMeta(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if s, ok := meta["location_code"].(string); ok {
		return s
	}
	return ""
}

func hostkeyStockIDFromMeta(meta map[string]any) int {
	if meta == nil {
		return 0
	}
	switch v := meta["stock_id"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func hostkeyPresetIDFromExt(ext string) (presetID int, location string) {
	presetID, location, _, _ = hostkey.ParseExternalProductID(ext)
	return presetID, location
}

func hostkeySourceFromMeta(meta map[string]any) string {
	if meta == nil {
		return "preset"
	}
	if s, ok := meta["source"].(string); ok && s != "" {
		return s
	}
	return "preset"
}
