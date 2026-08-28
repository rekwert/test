package hostkey

import "strings"

// portalHostkeyRegions matches dedicated location filters on the portal (UK → gb).
var portalHostkeyRegions = map[string]struct{}{
	"NL": {}, "DE": {}, "FI": {}, "UK": {}, "US": {}, "FR": {},
	"ES": {}, "PL": {}, "RU": {}, "IS": {}, "TR": {}, "IT": {}, "CH": {},
}

// EffectivePresetLocations merges Hostkey `locations` with priced keys from the price map.
// Invapi often omits RU/TR/IS from the locations string while still pricing those regions.
func EffectivePresetLocations(p Preset) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(p.Locations)+len(p.PriceByLoc))
	add := func(loc string) {
		loc = strings.ToUpper(strings.TrimSpace(loc))
		if loc == "" || loc == "ZZ" {
			return
		}
		if _, ok := portalHostkeyRegions[loc]; !ok {
			return
		}
		if _, ok := seen[loc]; ok {
			return
		}
		seen[loc] = struct{}{}
		out = append(out, loc)
	}
	for _, loc := range p.Locations {
		add(loc)
	}
	for loc, lp := range p.PriceByLoc {
		if lp.EUR > 0 || lp.RUB > 0 {
			add(loc)
		}
	}
	return out
}
