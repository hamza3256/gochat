//go:build linux

package poller

import (
	"syscall"
	"unsafe"
)

type epollPoller struct {
	epfd int
}

func New() (Poller, error) {
	fd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return &epollPoller{epfd: fd}, nil
}

func (p *epollPoller) Add(fd int) error {
	ev := syscall.EpollEvent{
		Events: syscall.EPOLLIN | syscall.EPOLLET,
		Fd:     int32(fd),
	}
	return syscall.EpollCtl(p.epfd, syscall.EPOLL_CTL_ADD, fd, &ev)
}

func (p *epollPoller) Mod(fd int, read, write bool) error {
	var flags uint32
	if read {
		flags |= syscall.EPOLLIN
	}
	if write {
		flags |= syscall.EPOLLOUT
	}
	flags |= syscall.EPOLLET
	ev := syscall.EpollEvent{
		Events: flags,
		Fd:     int32(fd),
	}
	return syscall.EpollCtl(p.epfd, syscall.EPOLL_CTL_MOD, fd, &ev)
}

func (p *epollPoller) Del(fd int) error {
	return syscall.EpollCtl(p.epfd, syscall.EPOLL_CTL_DEL, fd, nil)
}

func (p *epollPoller) Wait(events []Event, timeoutMs int) (int, error) {
	epollEvents := make([]syscall.EpollEvent, len(events))

	n, err := syscall.EpollWait(p.epfd, epollEvents, timeoutMs)
	if err != nil {
		if err == syscall.EINTR {
			return 0, nil
		}
		return 0, err
	}

	for i := 0; i < n; i++ {
		ee := &epollEvents[i]
		events[i] = Event{
			Fd:    int(ee.Fd),
			Read:  ee.Events&syscall.EPOLLIN != 0,
			Write: ee.Events&syscall.EPOLLOUT != 0,
		}
	}
	return n, nil
}

func (p *epollPoller) Close() error {
	return syscall.Close(p.epfd)
}

// Pad is unused but silences the "unsafe imported and not used" warning
// for builds that inline unsafe operations.
var _ = unsafe.Sizeof(0)
