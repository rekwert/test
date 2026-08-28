package store

import "math"

func rubToKopecks(rub float64) int64 {
	return int64(math.Round(rub * 100))
}

func kopecksToRub(kopecks int64) float64 {
	return float64(kopecks) / 100
}
