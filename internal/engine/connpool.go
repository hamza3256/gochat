package engine

import (
	"sync/atomic"

	"github.com/hamza3256/gochat/pkg/bytebuf"
)

type ConnPool struct {
	conns    []*Conn
	maxConns int
	active   atomic.Int64
	readSize int
}

func NewConnPool(maxConns, readBufSize int) *ConnPool {
	if maxConns < 64 {
		maxConns = 64
	}
	if readBufSize < 512 {
		readBufSize = 512
	}
	conns := make([]*Conn, maxConns)
	for i := range conns {
		conns[i] = &Conn{}
	}
	return &ConnPool{
		conns:    conns,
		maxConns: maxConns,
		readSize: readBufSize,
	}
}

func (p *ConnPool) Get(fd int) *Conn {
	if fd < 0 || fd >= p.maxConns {
		return nil
	}
	return p.conns[fd]
}

func (p *ConnPool) Activate(fd, reactor int) *Conn {
	c := p.Get(fd)
	if c == nil {
		return nil
	}
	c.Fd = fd
	c.Reactor = reactor
	c.RecvLen = 0
	c.RoomID = 0
	c.InRoom = false
	c.Upgraded = false
	c.OutQueue = c.OutQueue[:0]
	c.state.Store(connOpen)
	if c.ReadBuf == nil {
		c.ReadBuf = bytebuf.Get(p.readSize)
	}
	p.active.Add(1)
	return c
}

func (p *ConnPool) Deactivate(fd int) {
	c := p.Get(fd)
	if c == nil {
		return
	}
	c.markClosed()
	for _, buf := range c.OutQueue {
		bytebuf.Put(buf)
	}
	c.OutQueue = c.OutQueue[:0]
	c.RecvLen = 0
	p.active.Add(-1)
}

func (p *ConnPool) ActiveCount() int64 {
	return p.active.Load()
}

func (p *ConnPool) Cap() int {
	return p.maxConns
}
