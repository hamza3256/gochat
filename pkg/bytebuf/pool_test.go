package bytebuf

import (
	"testing"
)

func TestGetReturnsCorrectLength(t *testing.T) {
	for _, size := range []int{1, 63, 64, 65, 128, 1000, 4096, 65536} {
		buf := Get(size)
		if len(buf) != size {
			t.Errorf("Get(%d): len = %d, want %d", size, len(buf), size)
		}
		if cap(buf) < size {
			t.Errorf("Get(%d): cap = %d, want >= %d", size, cap(buf), size)
		}
		c := cap(buf)
		if c&(c-1) != 0 {
			t.Errorf("Get(%d): cap = %d, not power of 2", size, c)
		}
		Put(buf)
	}
}

func TestPutAndReuse(t *testing.T) {
	buf1 := Get(128)
	buf1[0] = 0xAB
	ptr1 := &buf1[:cap(buf1)][0]
	Put(buf1)

	buf2 := Get(128)
	ptr2 := &buf2[:cap(buf2)][0]
	if ptr1 != ptr2 {
		t.Log("buffer was not reused (may happen under GC pressure)")
	}
	Put(buf2)
}

func TestPutRejectsNonPowerOf2(t *testing.T) {
	buf := make([]byte, 100)
	Put(buf) // should not panic
}

func TestTierIndex(t *testing.T) {
	cases := []struct {
		size int
		tier int
	}{
		{1, 0},
		{64, 0},
		{65, 1},
		{128, 1},
		{129, 2},
		{256, 2},
	}
	for _, tc := range cases {
		got := tierIndex(tc.size)
		if got != tc.tier {
			t.Errorf("tierIndex(%d) = %d, want %d", tc.size, got, tc.tier)
		}
	}
}

func BenchmarkGetPut64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := Get(64)
		Put(buf)
	}
}

func BenchmarkGetPut4K(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := Get(4096)
		Put(buf)
	}
}

func BenchmarkGetPut64K(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := Get(65536)
		Put(buf)
	}
}
