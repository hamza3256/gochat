package hist

import "sort"

// SortInPlace sorts samples ascending so Percentile can be called.
func SortInPlace(samples []int64) {
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
}

// Percentile returns the nearest-rank percentile of a non-empty sorted slice.
// p is in [0, 100]. An empty slice returns 0.
func Percentile(sorted []int64, p float64) int64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[n-1]
	}
	rank := (p / 100) * float64(n-1)
	return sorted[int(rank+0.5)]
}
