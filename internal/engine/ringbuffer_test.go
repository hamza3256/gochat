package engine

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRingBufferBasic(t *testing.T) {
	rb := NewRingBuffer[int](8)
	if rb.Cap() != 8 {
		t.Fatalf("Cap() = %d, want 8", rb.Cap())
	}

	for i := 0; i < 8; i++ {
		if err := rb.Enqueue(i); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	if err := rb.Enqueue(99); err != ErrFull {
		t.Fatalf("expected ErrFull, got %v", err)
	}

	for i := 0; i < 8; i++ {
		v, err := rb.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue: %v", err)
		}
		if v != i {
			t.Fatalf("Dequeue = %d, want %d", v, i)
		}
	}

	if _, err := rb.Dequeue(); err != ErrEmpty {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := NewRingBuffer[int](4)
	for round := 0; round < 10; round++ {
		for i := 0; i < 4; i++ {
			if err := rb.Enqueue(round*4 + i); err != nil {
				t.Fatalf("round %d enqueue %d: %v", round, i, err)
			}
		}
		for i := 0; i < 4; i++ {
			v, err := rb.Dequeue()
			if err != nil {
				t.Fatalf("round %d dequeue %d: %v", round, i, err)
			}
			expected := round*4 + i
			if v != expected {
				t.Fatalf("got %d, want %d", v, expected)
			}
		}
	}
}

func TestRingBufferNonPow2(t *testing.T) {
	rb := NewRingBuffer[int](5)
	if rb.Cap() != 8 {
		t.Fatalf("Cap() = %d, want 8 (next pow2 of 5)", rb.Cap())
	}
}

func TestRingBufferConcurrentMPMC(t *testing.T) {
	const (
		capacity   = 1024
		producers  = 4
		consumers  = 4
		perWorker  = 5000
		totalItems = producers * perWorker
	)

	rb := NewRingBuffer[int](capacity)
	var consumed atomic.Int64
	var prodWg, consWg sync.WaitGroup
	var prodDone atomic.Bool

	seen := make([]atomic.Bool, totalItems)

	prodWg.Add(producers)
	for p := 0; p < producers; p++ {
		go func(id int) {
			defer prodWg.Done()
			base := id * perWorker
			for i := 0; i < perWorker; i++ {
				for rb.Enqueue(base+i) == ErrFull {
					runtime.Gosched()
				}
			}
		}(p)
	}

	consWg.Add(consumers)
	for c := 0; c < consumers; c++ {
		go func() {
			defer consWg.Done()
			for {
				v, err := rb.Dequeue()
				if err == ErrEmpty {
					if prodDone.Load() && rb.Len() == 0 {
						return
					}
					runtime.Gosched()
					continue
				}
				seen[v].Store(true)
				consumed.Add(1)
			}
		}()
	}

	prodWg.Wait()
	prodDone.Store(true)
	consWg.Wait()

	got := consumed.Load()
	if got != totalItems {
		t.Fatalf("consumed %d items, want %d", got, totalItems)
	}
	for i := 0; i < totalItems; i++ {
		if !seen[i].Load() {
			t.Fatalf("item %d was never consumed", i)
		}
	}
}

func TestRingBufferLen(t *testing.T) {
	rb := NewRingBuffer[int](8)
	if rb.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", rb.Len())
	}
	rb.Enqueue(1)
	rb.Enqueue(2)
	if rb.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", rb.Len())
	}
	rb.Dequeue()
	if rb.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", rb.Len())
	}
}

func BenchmarkRingBufferEnqueueDequeue(b *testing.B) {
	rb := NewRingBuffer[int](1024)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rb.Enqueue(i)
		_, _ = rb.Dequeue()
	}
}

func BenchmarkRingBufferMPMC(b *testing.B) {
	rb := NewRingBuffer[int](65536)
	b.SetParallelism(8)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				rb.Enqueue(i)
			} else {
				rb.Dequeue()
			}
			i++
		}
	})
}
