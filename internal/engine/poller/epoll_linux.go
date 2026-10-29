//go:build linux

package poller

import "syscall"

type epollPoller struct {
	epfd   int
	events []syscall.EpollEvent
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
		Events: syscall.EPOLLIN | syscall.EPOLLET | syscall.EPOLLRDHUP,
		Fd:     int32(fd),
	}
	return syscall.EpollCtl(p.epfd, syscall.EPOLL_CTL_ADD, fd, &ev)
}

func (p *epollPoller) Mod(fd int, read, write bool) error {
	var flags uint32 = syscall.EPOLLET | syscall.EPOLLRDHUP
	if read {
		flags |= syscall.EPOLLIN
	}
	if write {
		flags |= syscall.EPOLLOUT
	}
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
	if cap(p.events) < len(events) {
		p.events = make([]syscall.EpollEvent, len(events))
	} else {
		p.events = p.events[:len(events)]
	}

	n, err := syscall.EpollWait(p.epfd, p.events, timeoutMs)
	if err != nil {
		if err == syscall.EINTR {
			return 0, nil
		}
		return 0, err
	}

	for i := 0; i < n; i++ {
		ee := &p.events[i]
		events[i] = Event{
			Fd:     int(ee.Fd),
			Read:   ee.Events&syscall.EPOLLIN != 0,
			Write:  ee.Events&syscall.EPOLLOUT != 0,
			Hangup: ee.Events&(syscall.EPOLLERR|syscall.EPOLLHUP|syscall.EPOLLRDHUP) != 0,
		}
	}
	return n, nil
}

func (p *epollPoller) Close() error {
	return syscall.Close(p.epfd)
}
