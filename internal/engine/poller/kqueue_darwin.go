//go:build darwin

package poller

import (
	"syscall"
)

type kqueuePoller struct {
	kqfd    int
	changes []syscall.Kevent_t
}

func New() (Poller, error) {
	fd, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	return &kqueuePoller{
		kqfd:    fd,
		changes: make([]syscall.Kevent_t, 0, 64),
	}, nil
}

func (p *kqueuePoller) Add(fd int) error {
	changes := []syscall.Kevent_t{
		{
			Ident:  uint64(fd),
			Filter: syscall.EVFILT_READ,
			Flags:  syscall.EV_ADD | syscall.EV_CLEAR,
		},
	}
	_, err := syscall.Kevent(p.kqfd, changes, nil, nil)
	return err
}

func (p *kqueuePoller) Mod(fd int, read, write bool) error {
	p.changes = p.changes[:0]

	if read {
		p.changes = append(p.changes, syscall.Kevent_t{
			Ident:  uint64(fd),
			Filter: syscall.EVFILT_READ,
			Flags:  syscall.EV_ADD | syscall.EV_CLEAR,
		})
	} else {
		p.changes = append(p.changes, syscall.Kevent_t{
			Ident:  uint64(fd),
			Filter: syscall.EVFILT_READ,
			Flags:  syscall.EV_DELETE,
		})
	}

	if write {
		p.changes = append(p.changes, syscall.Kevent_t{
			Ident:  uint64(fd),
			Filter: syscall.EVFILT_WRITE,
			Flags:  syscall.EV_ADD | syscall.EV_CLEAR,
		})
	} else {
		p.changes = append(p.changes, syscall.Kevent_t{
			Ident:  uint64(fd),
			Filter: syscall.EVFILT_WRITE,
			Flags:  syscall.EV_DELETE,
		})
	}

	_, err := syscall.Kevent(p.kqfd, p.changes, nil, nil)
	return err
}

func (p *kqueuePoller) Del(fd int) error {
	changes := []syscall.Kevent_t{
		{
			Ident:  uint64(fd),
			Filter: syscall.EVFILT_READ,
			Flags:  syscall.EV_DELETE,
		},
		{
			Ident:  uint64(fd),
			Filter: syscall.EVFILT_WRITE,
			Flags:  syscall.EV_DELETE,
		},
	}
	// Ignore errors from deleting non-existent filters.
	syscall.Kevent(p.kqfd, changes, nil, nil)
	return nil
}

func (p *kqueuePoller) Wait(events []Event, timeoutMs int) (int, error) {
	kevents := make([]syscall.Kevent_t, len(events))

	var tp *syscall.Timespec
	if timeoutMs >= 0 {
		ts := syscall.NsecToTimespec(int64(timeoutMs) * 1e6)
		tp = &ts
	}

	n, err := syscall.Kevent(p.kqfd, nil, kevents, tp)
	if err != nil {
		if err == syscall.EINTR {
			return 0, nil
		}
		return 0, err
	}

	for i := 0; i < n; i++ {
		ke := &kevents[i]
		events[i] = Event{
			Fd:    int(ke.Ident),
			Read:  ke.Filter == syscall.EVFILT_READ,
			Write: ke.Filter == syscall.EVFILT_WRITE,
		}
	}
	return n, nil
}

func (p *kqueuePoller) Close() error {
	return syscall.Close(p.kqfd)
}
