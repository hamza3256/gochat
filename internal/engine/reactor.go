//go:build darwin || linux

package engine

import (
	"errors"
	"runtime"
	"sync/atomic"
	"syscall"

	"github.com/hamza3256/gochat/internal/engine/poller"
	"github.com/hamza3256/gochat/internal/ws"
	"github.com/hamza3256/gochat/pkg/bytebuf"
	"github.com/hamza3256/gochat/pkg/wire"
)

const maxRecvBuf = 1 << 20

type cmdKind uint8

const (
	cmdWrite cmdKind = iota
	cmdStop
)

type command struct {
	kind cmdKind
	fd   int
	data []byte
}

type Reactor struct {
	id         int
	poller     poller.Poller
	pool       *ConnPool
	rooms      *Rooms
	peers      []*Reactor
	metrics    *Metrics
	cmds       *RingBuffer[command]
	listenerFd int
	wakeR      int
	wakeW      int
	maxEvents  int
	running    atomic.Bool
	done       chan struct{}
}

func newReactor(
	id, listenerFd int,
	pool *ConnPool,
	rooms *Rooms,
	metrics *Metrics,
	cmdQueue uint64,
	maxEvents int,
) (*Reactor, error) {
	p, err := poller.New()
	if err != nil {
		return nil, err
	}
	if err := p.Add(listenerFd); err != nil {
		p.Close()
		return nil, err
	}

	var pipefd [2]int
	if err := syscall.Pipe(pipefd[:]); err != nil {
		p.Close()
		return nil, err
	}
	if err := syscall.SetNonblock(pipefd[0], true); err != nil {
		syscall.Close(pipefd[0])
		syscall.Close(pipefd[1])
		p.Close()
		return nil, err
	}
	if err := syscall.SetNonblock(pipefd[1], true); err != nil {
		syscall.Close(pipefd[0])
		syscall.Close(pipefd[1])
		p.Close()
		return nil, err
	}
	if err := p.Add(pipefd[0]); err != nil {
		syscall.Close(pipefd[0])
		syscall.Close(pipefd[1])
		p.Close()
		return nil, err
	}

	if maxEvents <= 0 {
		maxEvents = 1024
	}

	return &Reactor{
		id:         id,
		poller:     p,
		pool:       pool,
		rooms:      rooms,
		metrics:    metrics,
		cmds:       NewRingBuffer[command](cmdQueue),
		listenerFd: listenerFd,
		wakeR:      pipefd[0],
		wakeW:      pipefd[1],
		maxEvents:  maxEvents,
		done:       make(chan struct{}),
	}, nil
}

func (r *Reactor) Run() {
	defer close(r.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer r.cleanup()

	r.running.Store(true)
	events := make([]poller.Event, r.maxEvents)

	for r.running.Load() {
		n, err := r.poller.Wait(events, -1)
		if err != nil {
			if !r.running.Load() {
				return
			}
			continue
		}
		for i := 0; i < n; i++ {
			ev := &events[i]
			switch ev.Fd {
			case r.wakeR:
				r.drainWake()
			case r.listenerFd:
				r.acceptAll()
			default:
				if ev.Read {
					r.handleRead(ev.Fd)
				}
				if ev.Write {
					r.handleWrite(ev.Fd)
				}
				if ev.Hangup {
					r.closeConn(ev.Fd)
				}
			}
		}
		r.drainCmds()
	}
}

func (r *Reactor) Stop() {
	r.running.Store(false)
	_ = r.cmds.Enqueue(command{kind: cmdStop})
	r.wake()
}

func (r *Reactor) Wait() {
	<-r.done
}

func (r *Reactor) cleanup() {
	_ = r.poller.Close()
	_ = syscall.Close(r.listenerFd)
	_ = syscall.Close(r.wakeR)
	_ = syscall.Close(r.wakeW)
}

func (r *Reactor) wake() {
	var b [1]byte
	_, _ = syscall.Write(r.wakeW, b[:])
}

func (r *Reactor) drainWake() {
	var buf [128]byte
	for {
		n, err := syscall.Read(r.wakeR, buf[:])
		if n <= 0 || isAgain(err) {
			return
		}
	}
}

func (r *Reactor) drainCmds() {
	for {
		cmd, err := r.cmds.Dequeue()
		if err != nil {
			return
		}
		switch cmd.kind {
		case cmdWrite:
			r.writeRaw(cmd.fd, cmd.data)
		case cmdStop:
			r.running.Store(false)
		}
	}
}

func (r *Reactor) pushWrite(fd int, data []byte) {
	for {
		if r.cmds.Enqueue(command{kind: cmdWrite, fd: fd, data: data}) == nil {
			r.wake()
			return
		}
		runtime.Gosched()
	}
}

func (r *Reactor) acceptAll() {
	for {
		nfd, _, err := syscall.Accept(r.listenerFd)
		if err != nil {
			if isAgain(err) {
				return
			}
			return
		}
		if err := syscall.SetNonblock(nfd, true); err != nil {
			syscall.Close(nfd)
			continue
		}
		_ = syscall.SetsockoptInt(nfd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)

		conn := r.pool.Activate(nfd, r.id)
		if conn == nil {
			syscall.Close(nfd)
			continue
		}
		if err := r.poller.Add(nfd); err != nil {
			r.pool.Deactivate(nfd)
			syscall.Close(nfd)
			continue
		}
	}
}

func (r *Reactor) handleRead(fd int) {
	conn := r.pool.Get(fd)
	if conn == nil || conn.IsClosed() {
		return
	}

	for {
		n, err := syscall.Read(fd, conn.ReadBuf)
		if n > 0 {
			conn.appendRecv(conn.ReadBuf[:n])
			if conn.RecvLen > maxRecvBuf {
				r.closeConn(fd)
				return
			}
			if !r.consume(fd, conn) {
				return
			}
		}
		if err != nil {
			if isAgain(err) {
				return
			}
			r.closeConn(fd)
			return
		}
		if n == 0 {
			r.closeConn(fd)
			return
		}
	}
}

func (r *Reactor) consume(fd int, conn *Conn) bool {
	if !conn.Upgraded {
		req, n, err := ws.ParseUpgradeRequest(conn.RecvBuf[:conn.RecvLen])
		if err != nil {
			if err == ws.ErrIncomplete {
				return true
			}
			r.closeConn(fd)
			return false
		}
		copy(conn.RecvBuf, conn.RecvBuf[n:conn.RecvLen])
		conn.RecvLen -= n
		conn.Upgraded = true
		resp := ws.BuildUpgradeResponse(req.Key)
		buf := bytebuf.Get(len(resp))
		copy(buf, resp)
		r.writeRaw(fd, buf)
	}

	off := 0
	buf := conn.RecvBuf[:conn.RecvLen]
	for off < len(buf) {
		frame, n, err := ws.DecodeFrame(buf[off:])
		if err != nil {
			break
		}
		off += n
		if !r.handleFrame(fd, conn, frame) {
			return false
		}
	}
	if off > 0 {
		remaining := conn.RecvLen - off
		copy(conn.RecvBuf, conn.RecvBuf[off:conn.RecvLen])
		conn.RecvLen = remaining
	}
	return true
}

func (r *Reactor) handleFrame(fd int, conn *Conn, frame ws.Frame) bool {
	switch frame.Opcode {
	case ws.OpClose:
		r.closeConn(fd)
		return false
	case ws.OpPing:
		buf := bytebuf.Get(ws.FrameOverhead(len(frame.Payload)))
		n, err := ws.EncodeFrame(buf, ws.OpPong, frame.Payload)
		if err != nil {
			bytebuf.Put(buf)
			return true
		}
		r.writeRaw(fd, buf[:n])
		return true
	case ws.OpPong:
		return true
	case ws.OpBinary, ws.OpText:
		r.handleApp(fd, conn, frame.Payload)
		return true
	default:
		return true
	}
}

func (r *Reactor) handleApp(fd int, conn *Conn, payload []byte) {
	op, room, _, err := wire.Parse(payload)
	if err != nil {
		return
	}
	switch op {
	case wire.OpEcho:
		r.sendWS(fd, payload)
		if r.metrics != nil {
			r.metrics.Echoes.Add(1)
		}
	case wire.OpJoin:
		r.rooms.Join(room, Member{Fd: fd, ReactorID: r.id})
		conn.RoomID = room
		conn.InRoom = true
		if r.metrics != nil {
			r.metrics.Joins.Add(1)
		}
	case wire.OpPub:
		if r.metrics != nil {
			r.metrics.Pubs.Add(1)
		}
		members := r.rooms.Members(room)
		for _, m := range members {
			frame := r.encodeWS(payload)
			if frame == nil {
				continue
			}
			if m.ReactorID == r.id {
				r.writeRaw(m.Fd, frame)
				continue
			}
			if m.ReactorID >= 0 && m.ReactorID < len(r.peers) && r.peers[m.ReactorID] != nil {
				r.peers[m.ReactorID].pushWrite(m.Fd, frame)
			}
		}
	}
}

func (r *Reactor) sendWS(fd int, payload []byte) {
	frame := r.encodeWS(payload)
	if frame == nil {
		return
	}
	r.writeRaw(fd, frame)
}

func (r *Reactor) encodeWS(payload []byte) []byte {
	buf := bytebuf.Get(ws.FrameOverhead(len(payload)))
	n, err := ws.EncodeFrame(buf, ws.OpBinary, payload)
	if err != nil {
		bytebuf.Put(buf)
		return nil
	}
	return buf[:n]
}

func (r *Reactor) handleWrite(fd int) {
	conn := r.pool.Get(fd)
	if conn == nil || conn.IsClosed() {
		return
	}
	queue := conn.OutQueue
	conn.OutQueue = nil
	for _, data := range queue {
		r.writeRaw(fd, data)
		conn = r.pool.Get(fd)
		if conn == nil || conn.IsClosed() {
			return
		}
	}
	if conn != nil && len(conn.OutQueue) == 0 {
		_ = r.poller.Mod(fd, true, false)
	}
}

func (r *Reactor) writeRaw(fd int, data []byte) {
	conn := r.pool.Get(fd)
	if conn == nil || conn.IsClosed() {
		bytebuf.Put(data)
		return
	}
	if len(conn.OutQueue) > 0 {
		conn.OutQueue = append(conn.OutQueue, data)
		_ = r.poller.Mod(fd, true, true)
		return
	}

	written := 0
	for written < len(data) {
		n, err := syscall.Write(fd, data[written:])
		if n > 0 {
			written += n
		}
		if err != nil {
			if isAgain(err) {
				rest := bytebuf.Get(len(data) - written)
				copy(rest, data[written:])
				conn.OutQueue = append(conn.OutQueue, rest)
				bytebuf.Put(data)
				_ = r.poller.Mod(fd, true, true)
				return
			}
			bytebuf.Put(data)
			r.closeConn(fd)
			return
		}
		if n == 0 {
			bytebuf.Put(data)
			r.closeConn(fd)
			return
		}
	}
	bytebuf.Put(data)
}

func (r *Reactor) closeConn(fd int) {
	conn := r.pool.Get(fd)
	if conn == nil || conn.IsClosed() {
		return
	}
	_ = r.poller.Del(fd)
	if conn.InRoom {
		r.rooms.Leave(fd)
	}
	r.pool.Deactivate(fd)
	_ = syscall.Close(fd)
}

func isAgain(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}
