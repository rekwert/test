package money

import (
	"fmt"
	"math"
)

// Kopecks stores money as integer minor units (1 RUB = 100 kopecks).
type Kopecks int64

const KopecksPerRuble int64 = 100

// FromRubles converts rubles (float) to kopecks with bank rounding.
func FromRubles(rub float64) Kopecks {
	return Kopecks(math.Round(rub * float64(KopecksPerRuble)))
}

// ToRubles converts kopecks to rubles for API/display.
func (k Kopecks) ToRubles() float64 {
	return float64(k) / float64(KopecksPerRuble)
}

// String formats as RUB with 2 decimal places.
func (k Kopecks) String() string {
	return fmt.Sprintf("%.2f", k.ToRubles())
}

// Add returns sum of two amounts.
func (k Kopecks) Add(other Kopecks) Kopecks {
	return k + other
}

// Sub returns difference; caller must guard underflow.
func (k Kopecks) Sub(other Kopecks) Kopecks {
	return k - other
}
