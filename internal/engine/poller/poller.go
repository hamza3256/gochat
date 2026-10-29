package poller

import "errors"

type Event struct {
	Fd     int
	Read   bool
	Write  bool
	Hangup bool
}

type Poller interface {
	Add(fd int) error
	Mod(fd int, read, write bool) error
	Del(fd int) error
	Wait(events []Event, timeoutMs int) (n int, err error)
	Close() error
}

var (
	ErrClosed = errors.New("poller: closed")
	ErrBadFd  = errors.New("poller: bad file descriptor")
)
