package engine

import (
	"errors"
	"log"
	"runtime"
	"sync/atomic"
	"syscall"

	"github.com/hamza3256/gochat/internal/engine/poller"
	"github.com/hamza3256/gochat/pkg/bytebuf"
)

// ReactorConfig holds tunable parameters for a single reactor loop.
type ReactorConfig struct {
	MaxEvents    int
	ReadBufSize  int
	WriteBufSize int
}

// Reactor runs a single event loop on a dedicated OS thread, owning
// one kqueue/epoll instance and servicing I/O for its registered fds.
type Reactor struct {
	id       int
	poller   poller.Poller
	pool     *ConnPool
	ring     *RingBuffer[InternalEvent]
	listenerFd int
	cfg      ReactorConfig
	running  atomic.Bool
	onAccept func(fd int) // callback after accepting a new connection
	onClose  func(fd int) // callback after closing a connection
}

// InternalEvent is enqueued into the ring buffer by reactors for the
// dispatcher to process.
type InternalEvent struct {
	Type EventType
	Fd   int
	Data []byte
}

type EventType uint8

const (
	EventRead    EventType = 1
	EventClosed  EventType = 2
)

func NewReactor(
	id int,
	listenerFd int,
	pool *ConnPool,
	ring *RingBuffer[InternalEvent],
	cfg ReactorConfig,
) (*Reactor, error) {
	p, err := poller.New()
	if err != nil {
		return nil, err
	}

	if err := p.Add(listenerFd); err != nil {
		p.Close()
		return nil, err
	}

	return &Reactor{
		id:         id,
		poller:     p,
		pool:       pool,
		ring:       ring,
		listenerFd: listenerFd,
		cfg:        cfg,
	}, nil
}

// SetCallbacks sets optional lifecycle callbacks.
func (r *Reactor) SetCallbacks(onAccept, onClose func(fd int)) {
	r.onAccept = onAccept
	r.onClose = onClose
}

// Run starts the event loop. It locks to an OS thread and blocks
// until Stop is called.
func (r *Reactor) Run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	r.running.Store(true)
	events := make([]poller.Event, r.cfg.MaxEvents)

	for r.running.Load() {
		n, err := r.poller.Wait(events, 100) // 100ms timeout
		if err != nil {
			if r.running.Load() {
				log.Printf("reactor[%d] poll error: %v", r.id, err)
			}
			continue
		}

		for i := 0; i < n; i++ {
			ev := &events[i]
			if ev.Fd == r.listenerFd {
				r.acceptAll()
				continue
			}
			if ev.Read {
				r.handleRead(ev.Fd)
			}
			if ev.Write {
				r.handleWrite(ev.Fd)
			}
		}
	}
}

// Stop signals the reactor to exit after the current poll cycle.
func (r *Reactor) Stop() {
	r.running.Store(false)
	r.poller.Close()
}

func (r *Reactor) acceptAll() {
	for {
		nfd, _, err := syscall.Accept(r.listenerFd)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				return
			}
			log.Printf("reactor[%d] accept error: %v", r.id, err)
			return
		}

		if err := syscall.SetNonblock(nfd, true); err != nil {
			syscall.Close(nfd)
			continue
		}

		setTCPNoDelay(nfd)

		conn := r.pool.Activate(nfd)
		if conn == nil {
			syscall.Close(nfd)
			continue
		}

		if err := r.poller.Add(nfd); err != nil {
			r.pool.Deactivate(nfd)
			syscall.Close(nfd)
			continue
		}

		if r.onAccept != nil {
			r.onAccept(nfd)
		}
	}
}

func (r *Reactor) handleRead(fd int) {
	conn := r.pool.Get(fd)
	if conn == nil || conn.IsClosed() {
		return
	}

	buf := bytebuf.Get(r.cfg.ReadBufSize)
	for {
		n, err := syscall.Read(fd, buf)
		if n > 0 {
			data := bytebuf.Get(n)
			copy(data, buf[:n])
			evt := InternalEvent{
				Type: EventRead,
				Fd:   fd,
				Data: data,
			}
			if enqErr := r.ring.Enqueue(evt); enqErr != nil {
				bytebuf.Put(data)
			}
		}
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				break
			}
			r.closeConn(fd)
			break
		}
		if n == 0 {
			r.closeConn(fd)
			break
		}
	}
	bytebuf.Put(buf)
}

func (r *Reactor) handleWrite(fd int) {
	conn := r.pool.Get(fd)
	if conn == nil || conn.IsClosed() {
		return
	}

	bufs := conn.DrainOutQueue()
	for _, data := range bufs {
		written := 0
		for written < len(data) {
			n, err := syscall.Write(fd, data[written:])
			if err != nil {
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
					remaining := bytebuf.Get(len(data) - written)
					copy(remaining, data[written:])
					conn.EnqueueWrite(remaining)
					bytebuf.Put(data)
					r.poller.Mod(fd, true, true)
					return
				}
				r.closeConn(fd)
				bytebuf.Put(data)
				return
			}
			written += n
		}
		bytebuf.Put(data)
	}
	r.poller.Mod(fd, true, false)
}

// WriteToFd enqueues data for a connection and arms write interest.
func (r *Reactor) WriteToFd(fd int, data []byte) error {
	conn := r.pool.Get(fd)
	if conn == nil || conn.IsClosed() {
		return errors.New("connection closed")
	}
	conn.EnqueueWrite(data)
	return r.poller.Mod(fd, true, true)
}

func (r *Reactor) closeConn(fd int) {
	r.poller.Del(fd)
	r.pool.Deactivate(fd)
	syscall.Close(fd)

	r.ring.Enqueue(InternalEvent{Type: EventClosed, Fd: fd})

	if r.onClose != nil {
		r.onClose(fd)
	}
}

func setTCPNoDelay(fd int) {
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)
}

// NewListenerFd creates a non-blocking, reuse-port TCP listener and
// returns the raw file descriptor. The caller is responsible for
// closing it.
func NewListenerFd(addr string, backlog int) (int, error) {
	sa, err := resolveTCPAddr(addr)
	if err != nil {
		return -1, err
	}

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		return -1, err
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, soReusePort, 1); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return -1, err
	}

	if err := syscall.Bind(fd, sa); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err := syscall.Listen(fd, backlog); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

func resolveTCPAddr(addr string) (syscall.Sockaddr, error) {
	host := ""
	port := ""
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			host = addr[:i]
			port = addr[i+1:]
			break
		}
	}

	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}

	ip := [4]byte{0, 0, 0, 0}
	if host != "" && host != "0.0.0.0" {
		parts := splitIP(host)
		for i, part := range parts {
			v := 0
			for _, c := range part {
				v = v*10 + int(c-'0')
			}
			ip[i] = byte(v)
		}
	}

	sa := &syscall.SockaddrInet4{
		Port: p,
		Addr: ip,
	}
	return sa, nil
}

func splitIP(s string) [4]string {
	var parts [4]string
	idx := 0
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' {
			if idx < 4 {
				parts[idx] = s[start:i]
				idx++
			}
			start = i + 1
		}
	}
	return parts
}
