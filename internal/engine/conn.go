package engine

import "sync/atomic"

const (
	connOpen uint32 = iota
	connClosed
)

type Conn struct {
	Fd       int
	Reactor  int
	ReadBuf  []byte
	RecvBuf  []byte
	RecvLen  int
	OutQueue [][]byte
	RoomID   uint32
	InRoom   bool
	Upgraded bool
	state    atomic.Uint32
}

func (c *Conn) IsClosed() bool {
	return c.state.Load() == connClosed
}

func (c *Conn) markClosed() {
	c.state.Store(connClosed)
}

func (c *Conn) appendRecv(b []byte) {
	need := c.RecvLen + len(b)
	if cap(c.RecvBuf) < need {
		ncap := cap(c.RecvBuf) * 2
		if ncap < 4096 {
			ncap = 4096
		}
		for ncap < need {
			ncap *= 2
		}
		nbuf := make([]byte, ncap)
		copy(nbuf, c.RecvBuf[:c.RecvLen])
		c.RecvBuf = nbuf
	}
	copy(c.RecvBuf[c.RecvLen:], b)
	c.RecvLen = need
}
