package catalog

import "strings"

type OSTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Family      string   `json:"family"`
	SoftwareIDs []string `json:"software_ids,omitempty"`
}

type SoftwareProfile struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	InstallHint string            `json:"install_hint,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type Plan struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Tier         string  `json:"tier"`
	CPU          int     `json:"cpu"`
	RAMMb        int     `json:"ram_mb"`
	DiskGB       int     `json:"disk_gb"`
	PriceMonthly float64 `json:"price_monthly"`
	Region       string  `json:"region"`
	Active       bool    `json:"active"`
}

// Specs are RAM_GB / CPU / Disk_GB as sold in the portal.
// Legacy UUIDs …101–104 / …501–504 / …211–214 / …221–224 are kept for VirtFusion plan map continuity.
var plans = []Plan{
	// TRIAL — retired; free week is PROSTO-1 once per account.
	{ID: "11111111-1111-1111-1111-111111111601", Name: "TRIAL-1", Tier: "trial", CPU: 1, RAMMb: 2048, DiskGB: 10, PriceMonthly: 0, Region: "nl", Active: false},
	{ID: "11111111-1111-1111-1111-111111111602", Name: "TRIAL-1", Tier: "trial", CPU: 1, RAMMb: 2048, DiskGB: 10, PriceMonthly: 0, Region: "fi", Active: false},
	{ID: "11111111-1111-1111-1111-111111111603", Name: "TRIAL-1", Tier: "trial", CPU: 1, RAMMb: 2048, DiskGB: 10, PriceMonthly: 0, Region: "de", Active: false},
	{ID: "11111111-1111-1111-1111-111111111604", Name: "TRIAL-1", Tier: "trial", CPU: 1, RAMMb: 2048, DiskGB: 10, PriceMonthly: 0, Region: "gb", Active: false},

	// PROSTO — Xeon (model not shown in UI)
	{ID: "11111111-1111-1111-1111-111111111101", Name: "PROSTO-1", Tier: "prosto", CPU: 1, RAMMb: 1024, DiskGB: 10, PriceMonthly: 215, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111102", Name: "PROSTO-2", Tier: "prosto", CPU: 1, RAMMb: 2048, DiskGB: 30, PriceMonthly: 380, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111103", Name: "PROSTO-3", Tier: "prosto", CPU: 2, RAMMb: 4096, DiskGB: 50, PriceMonthly: 690, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111104", Name: "PROSTO-4", Tier: "prosto", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 990, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111105", Name: "PROSTO-5", Tier: "prosto", CPU: 4, RAMMb: 8192, DiskGB: 60, PriceMonthly: 1300, Region: "nl", Active: true},
	// PROSTO-6/7/8 — retired; max Prosto tier is PROSTO-5.
	{ID: "11111111-1111-1111-1111-111111111106", Name: "PROSTO-6", Tier: "prosto", CPU: 6, RAMMb: 12288, DiskGB: 80, PriceMonthly: 2199, Region: "nl", Active: false},
	{ID: "11111111-1111-1111-1111-111111111107", Name: "PROSTO-7", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 80, PriceMonthly: 3499, Region: "nl", Active: false},
	{ID: "11111111-1111-1111-1111-111111111108", Name: "PROSTO-8", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 4199, Region: "nl", Active: false},

	// PROSTO — Finland
	{ID: "11111111-1111-1111-1111-111111111501", Name: "PROSTO-1", Tier: "prosto", CPU: 1, RAMMb: 1024, DiskGB: 10, PriceMonthly: 215, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111502", Name: "PROSTO-2", Tier: "prosto", CPU: 1, RAMMb: 2048, DiskGB: 30, PriceMonthly: 380, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111503", Name: "PROSTO-3", Tier: "prosto", CPU: 2, RAMMb: 4096, DiskGB: 50, PriceMonthly: 690, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111504", Name: "PROSTO-4", Tier: "prosto", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 990, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111505", Name: "PROSTO-5", Tier: "prosto", CPU: 4, RAMMb: 8192, DiskGB: 60, PriceMonthly: 1300, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111506", Name: "PROSTO-6", Tier: "prosto", CPU: 6, RAMMb: 12288, DiskGB: 80, PriceMonthly: 2199, Region: "fi", Active: false},
	{ID: "11111111-1111-1111-1111-111111111507", Name: "PROSTO-7", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 80, PriceMonthly: 3499, Region: "fi", Active: false},
	{ID: "11111111-1111-1111-1111-111111111508", Name: "PROSTO-8", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 4199, Region: "fi", Active: false},

	// Midrange — EPYC 9354
	{ID: "11111111-1111-1111-1111-111111111211", Name: "Midrange-1", Tier: "midrange", CPU: 1, RAMMb: 2048, DiskGB: 40, PriceMonthly: 475, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111212", Name: "Midrange-2", Tier: "midrange", CPU: 2, RAMMb: 4096, DiskGB: 40, PriceMonthly: 890, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111213", Name: "Midrange-3", Tier: "midrange", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 1300, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111214", Name: "Midrange-4", Tier: "midrange", CPU: 4, RAMMb: 8192, DiskGB: 100, PriceMonthly: 1700, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111215", Name: "Midrange-5", Tier: "midrange", CPU: 6, RAMMb: 12288, DiskGB: 120, PriceMonthly: 2500, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111216", Name: "Midrange-6", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 150, PriceMonthly: 4800, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111217", Name: "Midrange-7", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 5400, Region: "nl", Active: false},

	// Midrange — Finland
	{ID: "11111111-1111-1111-1111-111111111511", Name: "Midrange-1", Tier: "midrange", CPU: 1, RAMMb: 2048, DiskGB: 40, PriceMonthly: 475, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111512", Name: "Midrange-2", Tier: "midrange", CPU: 2, RAMMb: 4096, DiskGB: 40, PriceMonthly: 890, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111513", Name: "Midrange-3", Tier: "midrange", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 1300, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111514", Name: "Midrange-4", Tier: "midrange", CPU: 4, RAMMb: 8192, DiskGB: 100, PriceMonthly: 1700, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111515", Name: "Midrange-5", Tier: "midrange", CPU: 6, RAMMb: 12288, DiskGB: 120, PriceMonthly: 2500, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111516", Name: "Midrange-6", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 150, PriceMonthly: 4800, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111517", Name: "Midrange-7", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 5400, Region: "fi", Active: false},

	// HUSTLE — Ryzen 9950X
	{ID: "11111111-1111-1111-1111-111111111221", Name: "HUSTLE-1", Tier: "hustle", CPU: 1, RAMMb: 2048, DiskGB: 100, PriceMonthly: 710, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111222", Name: "HUSTLE-2", Tier: "hustle", CPU: 2, RAMMb: 4096, DiskGB: 120, PriceMonthly: 1350, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111223", Name: "HUSTLE-3", Tier: "hustle", CPU: 4, RAMMb: 6144, DiskGB: 150, PriceMonthly: 2000, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111224", Name: "HUSTLE-4", Tier: "hustle", CPU: 4, RAMMb: 8192, DiskGB: 180, PriceMonthly: 2650, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111225", Name: "HUSTLE-5", Tier: "hustle", CPU: 6, RAMMb: 12288, DiskGB: 200, PriceMonthly: 3900, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111226", Name: "HUSTLE-6", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 250, PriceMonthly: 7900, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111227", Name: "HUSTLE-7", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 7800, Region: "nl", Active: false},

	// HUSTLE — Finland
	{ID: "11111111-1111-1111-1111-111111111521", Name: "HUSTLE-1", Tier: "hustle", CPU: 1, RAMMb: 2048, DiskGB: 100, PriceMonthly: 710, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111522", Name: "HUSTLE-2", Tier: "hustle", CPU: 2, RAMMb: 4096, DiskGB: 120, PriceMonthly: 1350, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111523", Name: "HUSTLE-3", Tier: "hustle", CPU: 4, RAMMb: 6144, DiskGB: 150, PriceMonthly: 2000, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111524", Name: "HUSTLE-4", Tier: "hustle", CPU: 4, RAMMb: 8192, DiskGB: 180, PriceMonthly: 2650, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111525", Name: "HUSTLE-5", Tier: "hustle", CPU: 6, RAMMb: 12288, DiskGB: 200, PriceMonthly: 3900, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111526", Name: "HUSTLE-6", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 250, PriceMonthly: 7900, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111527", Name: "HUSTLE-7", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 7800, Region: "fi", Active: false},

	// PROSTO — Germany (Frankfurt offer; sale gated by node availability)
	{ID: "11111111-1111-1111-1111-111111111701", Name: "PROSTO-1", Tier: "prosto", CPU: 1, RAMMb: 1024, DiskGB: 10, PriceMonthly: 215, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111702", Name: "PROSTO-2", Tier: "prosto", CPU: 1, RAMMb: 2048, DiskGB: 30, PriceMonthly: 380, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111703", Name: "PROSTO-3", Tier: "prosto", CPU: 2, RAMMb: 4096, DiskGB: 50, PriceMonthly: 690, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111704", Name: "PROSTO-4", Tier: "prosto", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 990, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111705", Name: "PROSTO-5", Tier: "prosto", CPU: 4, RAMMb: 8192, DiskGB: 60, PriceMonthly: 1300, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111706", Name: "PROSTO-6", Tier: "prosto", CPU: 6, RAMMb: 12288, DiskGB: 80, PriceMonthly: 2199, Region: "de", Active: false},
	{ID: "11111111-1111-1111-1111-111111111707", Name: "PROSTO-7", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 80, PriceMonthly: 3499, Region: "de", Active: false},
	{ID: "11111111-1111-1111-1111-111111111708", Name: "PROSTO-8", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 4199, Region: "de", Active: false},

	// PROSTO — United Kingdom
	{ID: "11111111-1111-1111-1111-111111111801", Name: "PROSTO-1", Tier: "prosto", CPU: 1, RAMMb: 1024, DiskGB: 10, PriceMonthly: 215, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111802", Name: "PROSTO-2", Tier: "prosto", CPU: 1, RAMMb: 2048, DiskGB: 30, PriceMonthly: 380, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111803", Name: "PROSTO-3", Tier: "prosto", CPU: 2, RAMMb: 4096, DiskGB: 50, PriceMonthly: 690, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111804", Name: "PROSTO-4", Tier: "prosto", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 990, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111805", Name: "PROSTO-5", Tier: "prosto", CPU: 4, RAMMb: 8192, DiskGB: 60, PriceMonthly: 1300, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111806", Name: "PROSTO-6", Tier: "prosto", CPU: 6, RAMMb: 12288, DiskGB: 80, PriceMonthly: 2199, Region: "gb", Active: false},
	{ID: "11111111-1111-1111-1111-111111111807", Name: "PROSTO-7", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 80, PriceMonthly: 3499, Region: "gb", Active: false},
	{ID: "11111111-1111-1111-1111-111111111808", Name: "PROSTO-8", Tier: "prosto", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 4199, Region: "gb", Active: false},

	// Midrange — Germany
	{ID: "11111111-1111-1111-1111-111111111711", Name: "Midrange-1", Tier: "midrange", CPU: 1, RAMMb: 2048, DiskGB: 40, PriceMonthly: 475, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111712", Name: "Midrange-2", Tier: "midrange", CPU: 2, RAMMb: 4096, DiskGB: 40, PriceMonthly: 890, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111713", Name: "Midrange-3", Tier: "midrange", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 1300, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111714", Name: "Midrange-4", Tier: "midrange", CPU: 4, RAMMb: 8192, DiskGB: 100, PriceMonthly: 1700, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111715", Name: "Midrange-5", Tier: "midrange", CPU: 6, RAMMb: 12288, DiskGB: 120, PriceMonthly: 2500, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111716", Name: "Midrange-6", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 150, PriceMonthly: 4800, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111717", Name: "Midrange-7", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 5400, Region: "de", Active: false},

	// Midrange — United Kingdom
	{ID: "11111111-1111-1111-1111-111111111811", Name: "Midrange-1", Tier: "midrange", CPU: 1, RAMMb: 2048, DiskGB: 40, PriceMonthly: 475, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111812", Name: "Midrange-2", Tier: "midrange", CPU: 2, RAMMb: 4096, DiskGB: 40, PriceMonthly: 890, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111813", Name: "Midrange-3", Tier: "midrange", CPU: 4, RAMMb: 6144, DiskGB: 60, PriceMonthly: 1300, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111814", Name: "Midrange-4", Tier: "midrange", CPU: 4, RAMMb: 8192, DiskGB: 100, PriceMonthly: 1700, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111815", Name: "Midrange-5", Tier: "midrange", CPU: 6, RAMMb: 12288, DiskGB: 120, PriceMonthly: 2500, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111816", Name: "Midrange-6", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 150, PriceMonthly: 4800, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111817", Name: "Midrange-7", Tier: "midrange", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 5400, Region: "gb", Active: false},

	// HUSTLE — Germany
	{ID: "11111111-1111-1111-1111-111111111721", Name: "HUSTLE-1", Tier: "hustle", CPU: 1, RAMMb: 2048, DiskGB: 100, PriceMonthly: 710, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111722", Name: "HUSTLE-2", Tier: "hustle", CPU: 2, RAMMb: 4096, DiskGB: 120, PriceMonthly: 1350, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111723", Name: "HUSTLE-3", Tier: "hustle", CPU: 4, RAMMb: 6144, DiskGB: 150, PriceMonthly: 2000, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111724", Name: "HUSTLE-4", Tier: "hustle", CPU: 4, RAMMb: 8192, DiskGB: 180, PriceMonthly: 2650, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111725", Name: "HUSTLE-5", Tier: "hustle", CPU: 6, RAMMb: 12288, DiskGB: 200, PriceMonthly: 3900, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111726", Name: "HUSTLE-6", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 250, PriceMonthly: 7900, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111727", Name: "HUSTLE-7", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 7800, Region: "de", Active: false},

	// HUSTLE — United Kingdom
	{ID: "11111111-1111-1111-1111-111111111821", Name: "HUSTLE-1", Tier: "hustle", CPU: 1, RAMMb: 2048, DiskGB: 100, PriceMonthly: 710, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111822", Name: "HUSTLE-2", Tier: "hustle", CPU: 2, RAMMb: 4096, DiskGB: 120, PriceMonthly: 1350, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111823", Name: "HUSTLE-3", Tier: "hustle", CPU: 4, RAMMb: 6144, DiskGB: 150, PriceMonthly: 2000, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111824", Name: "HUSTLE-4", Tier: "hustle", CPU: 4, RAMMb: 8192, DiskGB: 180, PriceMonthly: 2650, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111825", Name: "HUSTLE-5", Tier: "hustle", CPU: 6, RAMMb: 12288, DiskGB: 200, PriceMonthly: 3900, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111826", Name: "HUSTLE-6", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 250, PriceMonthly: 7900, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111827", Name: "HUSTLE-7", Tier: "hustle", CPU: 8, RAMMb: 24576, DiskGB: 120, PriceMonthly: 7800, Region: "gb", Active: false},

	// CUSTOM — Netherlands
	{ID: "11111111-1111-1111-1111-111111111901", Name: "CUSTOM-1", Tier: "custom", CPU: 2, RAMMb: 4096, DiskGB: 80, PriceMonthly: 1199, Region: "nl", Active: true},
	{ID: "11111111-1111-1111-1111-111111111902", Name: "CUSTOM-2", Tier: "custom", CPU: 4, RAMMb: 8192, DiskGB: 120, PriceMonthly: 0, Region: "nl", Active: false},

	// CUSTOM — Finland
	{ID: "11111111-1111-1111-1111-111111111911", Name: "CUSTOM-1", Tier: "custom", CPU: 2, RAMMb: 4096, DiskGB: 80, PriceMonthly: 1199, Region: "fi", Active: true},
	{ID: "11111111-1111-1111-1111-111111111912", Name: "CUSTOM-2", Tier: "custom", CPU: 4, RAMMb: 8192, DiskGB: 120, PriceMonthly: 0, Region: "fi", Active: false},

	// CUSTOM — Germany
	{ID: "11111111-1111-1111-1111-111111111921", Name: "CUSTOM-1", Tier: "custom", CPU: 2, RAMMb: 4096, DiskGB: 80, PriceMonthly: 1199, Region: "de", Active: true},
	{ID: "11111111-1111-1111-1111-111111111922", Name: "CUSTOM-2", Tier: "custom", CPU: 4, RAMMb: 8192, DiskGB: 120, PriceMonthly: 0, Region: "de", Active: false},

	// CUSTOM — United Kingdom
	{ID: "11111111-1111-1111-1111-111111111931", Name: "CUSTOM-1", Tier: "custom", CPU: 2, RAMMb: 4096, DiskGB: 80, PriceMonthly: 1199, Region: "gb", Active: true},
	{ID: "11111111-1111-1111-1111-111111111932", Name: "CUSTOM-2", Tier: "custom", CPU: 4, RAMMb: 8192, DiskGB: 120, PriceMonthly: 0, Region: "gb", Active: false},
}

var osTemplates = []OSTemplate{
	{ID: "alma-8", Name: "Alma Linux", Version: "8", Family: "rhel"},
	{ID: "alma-9", Name: "Alma Linux", Version: "9", Family: "rhel"},
	{ID: "astra-ce", Name: "Astra Linux", Version: "CE", Family: "rhel"},
	{ID: "centos-7", Name: "CentOS", Version: "7", Family: "rhel"},
	{ID: "centos-8-stream", Name: "CentOS", Version: "8 Stream", Family: "rhel"},
	{ID: "centos-9-stream", Name: "CentOS", Version: "9 Stream", Family: "rhel"},
	{ID: "debian-9", Name: "Debian", Version: "9", Family: "debian"},
	{ID: "debian-10", Name: "Debian", Version: "10", Family: "debian"},
	{ID: "debian-11", Name: "Debian", Version: "11", Family: "debian"},
	{ID: "debian-12", Name: "Debian", Version: "12", Family: "debian"},
	{ID: "freebsd-13", Name: "FreeBSD", Version: "13", Family: "freebsd"},
	{ID: "noos", Name: "NoOS", Version: "", Family: "none"},
	{ID: "oracle-8", Name: "Oracle Linux", Version: "8", Family: "rhel"},
	{ID: "oracle-9", Name: "Oracle Linux", Version: "9", Family: "rhel"},
	{ID: "rocky-8", Name: "Rocky Linux", Version: "8", Family: "rhel"},
	{ID: "ubuntu-16.04", Name: "Ubuntu", Version: "16.04", Family: "debian"},
	{ID: "ubuntu-18.04", Name: "Ubuntu", Version: "18.04", Family: "debian"},
	{ID: "ubuntu-20.04", Name: "Ubuntu", Version: "20.04 LTS", Family: "debian"},
	{ID: "ubuntu-22.04", Name: "Ubuntu", Version: "22.04 LTS", Family: "debian"},
	{ID: "ubuntu-24.04", Name: "Ubuntu", Version: "24.04 LTS", Family: "debian"},
}

var softwareProfiles = map[string]SoftwareProfile{
	"clean": {
		ID:          "clean",
		Name:        "Clean OS",
		Description: "Minimal OS install, no extra software",
		Labels:      map[string]string{"ru": "Чистая ОС", "en": "Clean OS"},
	},
	"3x-ui": {
		ID:          "3x-ui",
		Name:        "3X-UI",
		Description: "VPN из коробки: 3X-UI + VLESS (Reality, SNI www.5ka.ru) + Hysteria2 и первый клиент.",
		InstallHint: "После создания сервера ссылка на панель и конфиги VPN появятся в карточке сервера.",
		Labels:      map[string]string{"ru": "3X-UI VPN (VLESS + HY2)", "en": "3X-UI VPN (VLESS + HY2)"},
	},
	"marzban": {
		ID:          "marzban",
		Name:        "Marzban Xray",
		Description: "Marzban panel (Xray). Linux only.",
		InstallHint: "info in /root/info.txt",
		Labels:      map[string]string{"ru": "Marzban Xray", "en": "Marzban Xray"},
	},
	"python3": {
		ID:          "python3",
		Name:        "Python 3",
		Description: "Python 3 + pip + venv. Linux / FreeBSD.",
		InstallHint: "python3 информация в /root/install_info.txt",
		Labels:      map[string]string{"ru": "Python 3", "en": "Python 3"},
	},
	"claude-code": {
		ID:          "claude-code",
		Name:        "Claude Code",
		Description: "Claude Code CLI + web terminal with login/password. Ubuntu 20.04+, Debian 11+, Alma/Rocky 8+.",
		InstallHint: "После создания сервера в карточке появится ссылка на веб-терминал (логин/пароль). Затем в терминале: claude login.",
		Labels:      map[string]string{"ru": "Claude Code (веб-терминал)", "en": "Claude Code (web terminal)"},
	},
	"amnezia": {
		ID:          "amnezia",
		Name:        "Amnezia",
		Description: "Amnezia VPN (Docker): AmneziaWG на UDP 443, ссылка vpn:// для импорта в приложение Amnezia. Ubuntu 24.04 / Debian 12.",
		InstallHint: "После создания сервера в карточке появится ссылка vpn:// — вставьте её в приложение Amnezia VPN.",
		Labels:      map[string]string{"ru": "Amnezia", "en": "Amnezia"},
	},
}

var softwareByOS = map[string][]string{
	"alma-8":          {"clean", "3x-ui", "marzban", "python3"},
	"alma-9":          {"clean", "3x-ui", "marzban", "python3"},
	"astra-ce":        {"clean", "3x-ui", "marzban", "python3"},
	"centos-7":        {"clean", "3x-ui", "marzban", "python3"},
	"centos-8-stream": {"clean", "3x-ui", "marzban", "python3"},
	"centos-9-stream": {"clean", "3x-ui", "marzban", "python3"},
	"debian-9":        {"clean", "3x-ui", "marzban", "python3"},
	"debian-10":       {"clean", "3x-ui", "marzban", "python3"},
	"debian-11":       {"clean", "3x-ui", "marzban", "python3"},
	"debian-12":       {"clean", "3x-ui", "marzban", "python3"},
	"freebsd-13":      {"clean", "python3"},
	"noos":            {"clean"},
	"oracle-8":        {"clean", "3x-ui", "marzban", "python3"},
	"oracle-9":        {"clean", "3x-ui", "marzban", "python3"},
	"rocky-8":         {"clean", "3x-ui", "marzban", "python3"},
	"ubuntu-16.04":    {"clean", "3x-ui", "marzban", "python3"},
	"ubuntu-18.04":    {"clean", "3x-ui", "marzban", "python3"},
	"ubuntu-20.04":    {"clean", "3x-ui", "marzban", "python3"},
	"ubuntu-22.04":    {"clean", "3x-ui", "marzban", "python3"},
	"ubuntu-24.04":    {"clean", "3x-ui", "marzban", "python3"},
}

func Plans() []Plan {
	out := make([]Plan, len(plans))
	copy(out, plans)
	return out
}

func PlanByID(id string) (Plan, bool) {
	for _, p := range plans {
		if p.ID == id {
			return p, true
		}
	}
	return Plan{}, false
}

func FilterOSTemplatesByMap(ids map[string]int) []OSTemplate {
	if len(ids) == 0 {
		return OSTemplates()
	}
	out := make([]OSTemplate, 0, len(ids))
	for _, t := range osTemplates {
		if _, ok := ids[t.ID]; ok {
			out = append(out, t)
		}
	}
	return out
}

func OSTemplates() []OSTemplate {
	out := make([]OSTemplate, len(osTemplates))
	copy(out, osTemplates)
	return out
}

func SoftwareForOS(osID string) ([]SoftwareProfile, bool) {
	return profilesFromIDs(softwareIDsForOS(osID)), true
}

func SoftwareAllowed(osID, softwareID string) bool {
	softwareID = strings.TrimSpace(softwareID)
	if softwareID == "" || softwareID == "clean" {
		return true
	}
	for _, id := range softwareIDsForOS(osID) {
		if id == softwareID {
			return true
		}
	}
	return false
}

func CatalogResponse() map[string]any {
	return map[string]any{
		"os_templates":      EnrichOSTemplatesWithSoftware(OSTemplates()),
		"software_profiles": softwareProfilesList(),
	}
}

func softwareProfilesList() []SoftwareProfile {
	out := make([]SoftwareProfile, 0, len(softwareProfiles))
	for _, p := range softwareProfiles {
		out = append(out, p)
	}
	return out
}
