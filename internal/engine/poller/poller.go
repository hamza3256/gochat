package poller

import "errors"

// Event describes an I/O readiness notification for a file descriptor.
type Event struct {
	Fd    int
	Read  bool
	Write bool
}

// Poller abstracts the OS-specific I/O multiplexer (epoll on Linux,
// kqueue on macOS/BSD). All methods are NOT safe for concurrent use;
// each reactor goroutine owns exactly one Poller.
type Poller interface {
	// Add registers a file descriptor for read-readiness monitoring.
	Add(fd int) error

	// Mod changes the interest set for an already-registered fd.
	Mod(fd int, read, write bool) error

	// Del removes a file descriptor from the poller.
	Del(fd int) error

	// Wait blocks until at least one event fires or the timeout
	// (in milliseconds) expires. It fills events[:n] and returns n.
	// A negative timeout blocks indefinitely.
	Wait(events []Event, timeoutMs int) (n int, err error)

	// Close releases the poller's file descriptor.
	Close() error
}

var (
	ErrClosed     = errors.New("poller: closed")
	ErrBadFd      = errors.New("poller: bad file descriptor")
)
