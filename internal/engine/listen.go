//go:build darwin || linux

package engine

import (
	"fmt"
	"strconv"
	"syscall"
)

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

func BoundAddr(fd int) (string, error) {
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		return "", err
	}
	v, ok := sa.(*syscall.SockaddrInet4)
	if !ok {
		return "", fmt.Errorf("listen: not ipv4")
	}
	return fmt.Sprintf("%d.%d.%d.%d:%d", v.Addr[0], v.Addr[1], v.Addr[2], v.Addr[3], v.Port), nil
}

func ListenMany(addr string, n, backlog int) (fds []int, bound string, err error) {
	if n < 1 {
		n = 1
	}
	fd, err := NewListenerFd(addr, backlog)
	if err != nil {
		return nil, "", err
	}
	bound, err = BoundAddr(fd)
	if err != nil {
		syscall.Close(fd)
		return nil, "", err
	}
	fds = append(fds, fd)
	for i := 1; i < n; i++ {
		fd, err = NewListenerFd(bound, backlog)
		if err != nil {
			for _, old := range fds {
				syscall.Close(old)
			}
			return nil, "", err
		}
		fds = append(fds, fd)
	}
	return fds, bound, nil
}

func resolveTCPAddr(addr string) (syscall.Sockaddr, error) {
	host := ""
	portStr := ""
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			host = addr[:i]
			portStr = addr[i+1:]
			break
		}
	}
	if portStr == "" {
		return nil, fmt.Errorf("listen: missing port in %q", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return nil, fmt.Errorf("listen: bad port %q", portStr)
	}

	ip := [4]byte{0, 0, 0, 0}
	if host != "" && host != "0.0.0.0" {
		parts := splitIP(host)
		for i, part := range parts {
			v, convErr := strconv.Atoi(part)
			if convErr != nil || v < 0 || v > 255 {
				return nil, fmt.Errorf("listen: bad ipv4 host %q", host)
			}
			ip[i] = byte(v)
		}
	}

	return &syscall.SockaddrInet4{Port: port, Addr: ip}, nil
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

func dialHost(bound string) string {
	host := "127.0.0.1"
	port := bound
	for i := len(bound) - 1; i >= 0; i-- {
		if bound[i] == ':' {
			h := bound[:i]
			port = bound[i+1:]
			if h != "" && h != "0.0.0.0" {
				host = h
			}
			break
		}
	}
	return host + ":" + port
}
