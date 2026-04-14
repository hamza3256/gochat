package engine

import (
	"errors"
	"sync/atomic"
	"unsafe"
)

// CacheLineSize is the typical x86-64 cache line width.
const CacheLineSize = 64

var (
	ErrFull  = errors.New("ringbuffer: full")
	ErrEmpty = errors.New("ringbuffer: empty")
)

// slot holds one element in the ring buffer together with a sequence
// counter used for lock-free coordination.
type slot[T any] struct {
	seq   atomic.Uint64
	value T
}

// RingBuffer is a bounded, lock-free, multi-producer multi-consumer
// queue. The capacity must be a power of two.
//
// The implementation follows the Dmitry Vyukov MPMC queue design:
// each slot carries a sequence number. Producers CAS-advance the tail
// and wait until the target slot's sequence equals the expected
// position. Consumers CAS-advance the head analogously.
//
// Head and tail are placed on separate cache lines to prevent false
// sharing between producers and consumers.
type RingBuffer[T any] struct {
	mask uint64
	_    [CacheLineSize - unsafe.Sizeof(uint64(0))]byte
	head atomic.Uint64
	_    [CacheLineSize - unsafe.Sizeof(atomic.Uint64{})]byte
	tail atomic.Uint64
	_    [CacheLineSize - unsafe.Sizeof(atomic.Uint64{})]byte

	slots []slot[T]
}

// NewRingBuffer creates a ring buffer that can hold `capacity` items.
// capacity is rounded up to the next power of two if it isn't one.
func NewRingBuffer[T any](capacity uint64) *RingBuffer[T] {
	capacity = nextPow2(capacity)
	rb := &RingBuffer[T]{
		mask:  capacity - 1,
		slots: make([]slot[T], capacity),
	}
	for i := uint64(0); i < capacity; i++ {
		rb.slots[i].seq.Store(i)
	}
	return rb
}

// Enqueue adds an item to the ring buffer. Returns ErrFull if the
// buffer has no free slots (non-blocking).
func (rb *RingBuffer[T]) Enqueue(item T) error {
	for {
		tail := rb.tail.Load()
		s := &rb.slots[tail&rb.mask]
		seq := s.seq.Load()
		diff := int64(seq) - int64(tail)

		if diff == 0 {
			if rb.tail.CompareAndSwap(tail, tail+1) {
				s.value = item
				s.seq.Store(tail + 1)
				return nil
			}
		} else if diff < 0 {
			return ErrFull
		}
		// diff > 0: another producer won the CAS, retry.
	}
}

// Dequeue removes and returns the next item. Returns ErrEmpty if the
// buffer is empty (non-blocking).
func (rb *RingBuffer[T]) Dequeue() (T, error) {
	for {
		head := rb.head.Load()
		s := &rb.slots[head&rb.mask]
		seq := s.seq.Load()
		diff := int64(seq) - int64(head+1)

		if diff == 0 {
			if rb.head.CompareAndSwap(head, head+1) {
				item := s.value
				var zero T
				s.value = zero
				s.seq.Store(head + rb.mask + 1)
				return item, nil
			}
		} else if diff < 0 {
			var zero T
			return zero, ErrEmpty
		}
		// diff > 0: another consumer won the CAS, retry.
	}
}

// Len returns an approximate count of items currently in the buffer.
func (rb *RingBuffer[T]) Len() uint64 {
	tail := rb.tail.Load()
	head := rb.head.Load()
	if tail >= head {
		return tail - head
	}
	return 0
}

// Cap returns the ring buffer capacity.
func (rb *RingBuffer[T]) Cap() uint64 {
	return rb.mask + 1
}

func nextPow2(v uint64) uint64 {
	if v == 0 {
		return 1
	}
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	return v + 1
}
