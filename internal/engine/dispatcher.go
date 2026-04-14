package engine

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/hamza3256/gochat/pkg/bytebuf"
)

// MessageSink processes decoded messages from the dispatcher. This is
// the bridge between the engine layer and the application service layer.
type MessageSink interface {
	OnData(fd int, data []byte) error
	OnDisconnect(fd int)
}

// DispatcherConfig configures the worker pool.
type DispatcherConfig struct {
	Workers int
}

// Dispatcher drains InternalEvents from the ring buffer using a pool
// of worker goroutines and forwards them to the MessageSink.
type Dispatcher struct {
	ring    *RingBuffer[InternalEvent]
	sink    MessageSink
	workers int
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	running atomic.Bool

	processed atomic.Uint64
}

func NewDispatcher(ring *RingBuffer[InternalEvent], sink MessageSink, cfg DispatcherConfig) *Dispatcher {
	workers := cfg.Workers
	if workers <= 0 {
		workers = runtime.NumCPU() * 4
	}
	return &Dispatcher{
		ring:    ring,
		sink:    sink,
		workers: workers,
	}
}

// Start launches the worker goroutines. Non-blocking.
func (d *Dispatcher) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.running.Store(true)

	d.wg.Add(d.workers)
	for i := 0; i < d.workers; i++ {
		go d.worker(ctx)
	}
}

// Stop signals all workers and waits for them to drain.
func (d *Dispatcher) Stop() {
	d.running.Store(false)
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
}

// Processed returns the total number of events dispatched.
func (d *Dispatcher) Processed() uint64 {
	return d.processed.Load()
}

func (d *Dispatcher) worker(ctx context.Context) {
	defer d.wg.Done()

	for {
		select {
		case <-ctx.Done():
			d.drainRemaining()
			return
		default:
		}

		evt, err := d.ring.Dequeue()
		if err == ErrEmpty {
			runtime.Gosched()
			continue
		}

		d.dispatch(evt)
	}
}

func (d *Dispatcher) dispatch(evt InternalEvent) {
	switch evt.Type {
	case EventRead:
		if d.sink != nil {
			d.sink.OnData(evt.Fd, evt.Data)
		}
		if evt.Data != nil {
			bytebuf.Put(evt.Data)
		}
	case EventClosed:
		if d.sink != nil {
			d.sink.OnDisconnect(evt.Fd)
		}
	}
	d.processed.Add(1)
}

func (d *Dispatcher) drainRemaining() {
	for {
		evt, err := d.ring.Dequeue()
		if err == ErrEmpty {
			return
		}
		d.dispatch(evt)
	}
}
