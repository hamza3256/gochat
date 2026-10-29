package hist

import "testing"

func TestPercentileKnownSet(t *testing.T) {
	samples := []int64{5, 1, 4, 2, 3}
	SortInPlace(samples)
	if want := []int64{1, 2, 3, 4, 5}; !equal(samples, want) {
		t.Fatalf("sorted=%v, want %v", samples, want)
	}
	if got := Percentile(samples, 0); got != 1 {
		t.Fatalf("p0=%d, want 1", got)
	}
	if got := Percentile(samples, 50); got != 3 {
		t.Fatalf("p50=%d, want 3", got)
	}
	if got := Percentile(samples, 100); got != 5 {
		t.Fatalf("p100=%d, want 5", got)
	}
}

func TestPercentileEmpty(t *testing.T) {
	if got := Percentile(nil, 50); got != 0 {
		t.Fatalf("empty=%d, want 0", got)
	}
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
