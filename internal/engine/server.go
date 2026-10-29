//go:build darwin || linux

package engine

import (
	"fmt"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
)

type Config struct {
	Addr        string
	NumReactors int
	MaxConns    int
	ReadBufSize int
	Backlog     int
	CmdQueue    uint64
	MaxEvents   int
}

type Metrics struct {
	Echoes atomic.Uint64
	Pubs   atomic.Uint64
	Joins  atomic.Uint64
}

type Snapshot struct {
	Connections int64
	Echoes      uint64
	Pubs        uint64
	Joins       uint64
}

type Server struct {
	cfg      Config
	pool     *ConnPool
	rooms    *Rooms
	metrics  *Metrics
	reactors []*Reactor
	bound    string
	started  atomic.Bool
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	if cfg.NumReactors <= 0 {
		cfg.NumReactors = runtime.NumCPU()
	}
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = 65536
	}
	if cfg.ReadBufSize <= 0 {
		cfg.ReadBufSize = 4096
	}
	if cfg.Backlog <= 0 {
		cfg.Backlog = 4096
	}
	if cfg.CmdQueue == 0 {
		cfg.CmdQueue = 65536
	}

	fds, bound, err := ListenMany(cfg.Addr, cfg.NumReactors, cfg.Backlog)
	if err != nil {
		return nil, err
	}

	pool := NewConnPool(cfg.MaxConns, cfg.ReadBufSize)
	rooms := NewRooms()
	metrics := &Metrics{}
	reactors := make([]*Reactor, cfg.NumReactors)
	for i := 0; i < cfg.NumReactors; i++ {
		r, err := newReactor(i, fds[i], pool, rooms, metrics, cfg.CmdQueue, cfg.MaxEvents)
		if err != nil {
			for j := 0; j < i; j++ {
				reactors[j].cleanup()
			}
			for j := i; j < len(fds); j++ {
				syscall.Close(fds[j])
			}
			return nil, err
		}
		reactors[i] = r
	}
	for _, r := range reactors {
		r.peers = reactors
	}

	return &Server{
		cfg:      cfg,
		pool:     pool,
		rooms:    rooms,
		metrics:  metrics,
		reactors: reactors,
		bound:    bound,
	}, nil
}

func (s *Server) Start() {
	if !s.started.CompareAndSwap(false, true) {
		return
	}
	signal.Ignore(syscall.SIGPIPE)
	for _, r := range s.reactors {
		go r.Run()
	}
}

func (s *Server) Close() {
	if !s.started.Load() {
		for _, r := range s.reactors {
			r.cleanup()
		}
		return
	}
	for _, r := range s.reactors {
		r.Stop()
	}
	for _, r := range s.reactors {
		r.Wait()
	}
}

func (s *Server) Addr() string {
	return dialHost(s.bound)
}

func (s *Server) Bound() string {
	return s.bound
}

func (s *Server) Snapshot() Snapshot {
	return Snapshot{
		Connections: s.pool.ActiveCount(),
		Echoes:      s.metrics.Echoes.Load(),
		Pubs:        s.metrics.Pubs.Load(),
		Joins:       s.metrics.Joins.Load(),
	}
}

func (s *Server) String() string {
	st := s.Snapshot()
	return fmt.Sprintf("addr=%s conns=%d echoes=%d pubs=%d joins=%d",
		s.Addr(), st.Connections, st.Echoes, st.Pubs, st.Joins)
}
