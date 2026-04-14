package bytebuf

import (
	"math/bits"
	"sync"
	"sync/atomic"
)

const (
	minShift = 6  // 64 B
	maxShift = 25 // 32 MB
	numTiers = maxShift - minShift + 1
)

var (
	pools [numTiers]sync.Pool
	misses atomic.Uint64
)

func init() {
	for i := range pools {
		size := 1 << (i + minShift)
		pools[i].New = func() any {
			misses.Add(1)
			buf := make([]byte, size)
			return &buf
		}
	}
}

func tierIndex(size int) int {
	if size <= 1<<minShift {
		return 0
	}
	idx := bits.Len(uint(size-1)) - minShift
	if idx >= numTiers {
		return numTiers - 1
	}
	return idx
}

// Get returns a byte slice of at least the requested size from the
// smallest sufficient pool tier. The returned slice has length == size
// and capacity rounded up to the tier boundary.
func Get(size int) []byte {
	idx := tierIndex(size)
	bp := pools[idx].Get().(*[]byte)
	buf := (*bp)[:size]
	return buf
}

// Put returns a buffer to its pool tier. Buffers not originating from
// Get (or whose capacity doesn't match a tier) are silently dropped.
func Put(buf []byte) {
	c := cap(buf)
	if c == 0 || c&(c-1) != 0 {
		return
	}
	shift := bits.TrailingZeros(uint(c))
	idx := shift - minShift
	if idx < 0 || idx >= numTiers {
		return
	}
	buf = buf[:c]
	pools[idx].Put(&buf)
}

// Misses returns the cumulative number of pool cache misses (allocations).
func Misses() uint64 {
	return misses.Load()
}
