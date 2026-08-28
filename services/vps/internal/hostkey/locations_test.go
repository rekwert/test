package hostkey

import "testing"

func TestEffectivePresetLocationsMergesPriceMap(t *testing.T) {
	p := Preset{
		Locations: []string{"NL", "DE"},
		PriceByLoc: map[string]LocationPrice{
			"NL": {EUR: 50, RUB: 5000},
			"RU": {RUB: 4500},
			"TR": {EUR: 55},
			"ZZ": {EUR: 1},
		},
	}
	got := EffectivePresetLocations(p)
	want := map[string]bool{"NL": true, "DE": true, "RU": true, "TR": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d locations", got, len(want))
	}
	for _, loc := range got {
		if !want[loc] {
			t.Fatalf("unexpected location %q in %v", loc, got)
		}
	}
}

func TestEffectivePresetLocationsSkipsUnpricedExtraKeys(t *testing.T) {
	p := Preset{
		Locations:  []string{"NL"},
		PriceByLoc: map[string]LocationPrice{"RU": {}},
	}
	got := EffectivePresetLocations(p)
	if len(got) != 1 || got[0] != "NL" {
		t.Fatalf("got %v", got)
	}
}
