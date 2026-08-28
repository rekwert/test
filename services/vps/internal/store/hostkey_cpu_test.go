package store

import "testing"

func TestHostkeyCPUFromBMLine(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"BM E3-12xx/32/2x960GB SSD", "Intel Xeon E3-12xx"},
		{"BM 2xE5-2680v4/128/2x960", "2× Intel Xeon E5-2680v4"},
		{"BM i9-9900K/64/1TB NVMe + RTX A5000", "Intel Core i9-9900K"},
		{"BM 2xEPYC 7551/256/2x1.92TB SSD", "2× AMD EPYC 7551"},
		{"BM Ryzen 5950x/128/2x1TB nvme", "AMD Ryzen 5950x"},
		{"4 Cores 3.2/3.8Ghz", ""},
	}
	for _, tc := range tests {
		got := hostkeyCPUFromBMLine(tc.in)
		if got != tc.want {
			t.Errorf("hostkeyCPUFromBMLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHostkeyIsGenericCoreLabel(t *testing.T) {
	if !hostkeyIsGenericCoreLabel("4 Cores 3.2/3.8Ghz") {
		t.Fatal("expected generic core label")
	}
	if hostkeyIsGenericCoreLabel("Intel Xeon E5-2680v4") {
		t.Fatal("expected named CPU")
	}
}
