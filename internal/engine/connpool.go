package engine

import (
	"sync"
	"sync/atomic"

	"github.com/hamza3256/gochat/internal/domain"
	"github.com/hamza3256/gochat/pkg/bytebuf"
)

// ConnState tracks the lifecycle of a connection.
type ConnState uint32

const (
	ConnStateOpen   ConnState = 0
	ConnStateClosed ConnState = 1
)

// Conn is a lightweight representation of a single WebSocket
// connection, designed for slab allocation indexed by file descriptor.
type Conn struct {
	Fd       int
	Session  domain.Session
	ReadBuf  []byte
	WriteBuf []byte
	OutQueue [][]byte
	State    atomic.Uint32
	mu       sync.Mutex // protects OutQueue and WriteBuf
}

func (c *Conn) IsClosed() bool {
	return ConnState(c.State.Load()) == ConnStateClosed
}

func (c *Conn) MarkClosed() {
	c.State.Store(uint32(ConnStateClosed))
}

// EnqueueWrite appends data to the outbound queue. Thread-safe.
func (c *Conn) EnqueueWrite(data []byte) {
	c.mu.Lock()
	c.OutQueue = append(c.OutQueue, data)
	c.mu.Unlock()
}

// DrainOutQueue returns and clears all pending outbound data. Thread-safe.
func (c *Conn) DrainOutQueue() [][]byte {
	c.mu.Lock()
	q := c.OutQueue
	c.OutQueue = nil
	c.mu.Unlock()
	return q
}

// Reset clears the connection for reuse (when the fd slot is recycled).
func (c *Conn) Reset(fd int) {
	c.Fd = fd
	c.Session = domain.Session{}
	c.State.Store(uint32(ConnStateOpen))
	if c.ReadBuf != nil {
		bytebuf.Put(c.ReadBuf)
	}
	if c.WriteBuf != nil {
		bytebuf.Put(c.WriteBuf)
	}
	c.ReadBuf = nil
	c.WriteBuf = nil
	c.OutQueue = c.OutQueue[:0]
}

// ConnPool is a pre-allocated, slab-style connection registry indexed
// by file descriptor. It avoids map overhead for millions of entries.
type ConnPool struct {
	conns     []*Conn
	maxConns  int
	active    atomic.Int64
	readSize  int
	writeSize int
}

func NewConnPool(maxConns, readBufSize, writeBufSize int) *ConnPool {
	conns := make([]*Conn, maxConns)
	for i := range conns {
		conns[i] = &Conn{
			OutQueue: make([][]byte, 0, 8),
		}
	}
	return &ConnPool{
		conns:     conns,
		maxConns:  maxConns,
		readSize:  readBufSize,
		writeSize: writeBufSize,
	}
}

// Get returns the connection slot for the given fd, initializing its
// buffers if necessary. Returns nil if fd is out of range.
func (p *ConnPool) Get(fd int) *Conn {
	if fd < 0 || fd >= p.maxConns {
		return nil
	}
	return p.conns[fd]
}

// Activate prepares a slot for a new connection.
func (p *ConnPool) Activate(fd int) *Conn {
	c := p.Get(fd)
	if c == nil {
		return nil
	}
	c.Reset(fd)
	c.ReadBuf = bytebuf.Get(p.readSize)
	c.WriteBuf = bytebuf.Get(p.writeSize)
	p.active.Add(1)
	return c
}

// Deactivate marks a connection closed and recycles its buffers.
func (p *ConnPool) Deactivate(fd int) {
	c := p.Get(fd)
	if c == nil {
		return
	}
	c.MarkClosed()
	if c.ReadBuf != nil {
		bytebuf.Put(c.ReadBuf)
		c.ReadBuf = nil
	}
	if c.WriteBuf != nil {
		bytebuf.Put(c.WriteBuf)
		c.WriteBuf = nil
	}
	for _, buf := range c.OutQueue {
		bytebuf.Put(buf)
	}
	c.OutQueue = c.OutQueue[:0]
	p.active.Add(-1)
}

// ActiveCount returns the number of currently active connections.
func (p *ConnPool) ActiveCount() int64 {
	return p.active.Load()
}

// Cap returns the maximum number of connections supported.
func (p *ConnPool) Cap() int {
	return p.maxConns
}
