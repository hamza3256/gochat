package wsclient

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/hamza3256/gochat/internal/ws"
)

type Conn struct {
	nc   net.Conn
	recv []byte
}

func Dial(addr string) (*Conn, error) {
	addr = strings.TrimPrefix(addr, "ws://")
	addr = strings.TrimPrefix(addr, "http://")
	if i := strings.Index(addr, "/"); i >= 0 {
		addr = addr[:i]
	}

	nc, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	if tc, ok := nc.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	_ = nc.SetDeadline(time.Now().Add(5 * time.Second))

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		nc.Close()
		return nil, err
	}
	secKey := base64.StdEncoding.EncodeToString(key)
	req := fmt.Sprintf(
		"GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		addr, secKey,
	)
	if _, err := nc.Write([]byte(req)); err != nil {
		nc.Close()
		return nil, err
	}

	var raw []byte
	tmp := make([]byte, 1024)
	for {
		n, err := nc.Read(tmp)
		if n > 0 {
			raw = append(raw, tmp[:n]...)
		}
		if idx := strings.Index(string(raw), "\r\n\r\n"); idx >= 0 {
			body := string(raw)
			if !strings.Contains(body, " 101 ") {
				nc.Close()
				return nil, fmt.Errorf("wsclient: upgrade failed: %q", trim(body, 120))
			}
			_ = nc.SetDeadline(time.Time{})
			return &Conn{nc: nc, recv: append([]byte(nil), raw[idx+4:]...)}, nil
		}
		if err != nil {
			nc.Close()
			return nil, err
		}
	}
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.nc.SetDeadline(t)
}

func (c *Conn) WriteBinary(payload []byte) error {
	buf := make([]byte, ws.FrameOverheadMasked(len(payload)))
	n, err := ws.EncodeFrameMasked(buf, ws.OpBinary, payload)
	if err != nil {
		return err
	}
	_, err = c.nc.Write(buf[:n])
	return err
}

func (c *Conn) ReadBinary() ([]byte, error) {
	for {
		frame, n, err := ws.DecodeFrame(c.recv)
		if err == nil {
			c.recv = c.recv[n:]
			switch frame.Opcode {
			case ws.OpBinary, ws.OpText:
				out := make([]byte, len(frame.Payload))
				copy(out, frame.Payload)
				return out, nil
			case ws.OpClose:
				return nil, io.EOF
			case ws.OpPing, ws.OpPong, ws.OpContinuation:
				continue
			default:
				continue
			}
		}
		if err != io.ErrUnexpectedEOF {
			return nil, err
		}
		var chunk [4096]byte
		n, rerr := c.nc.Read(chunk[:])
		if n > 0 {
			c.recv = append(c.recv, chunk[:n]...)
		}
		if rerr != nil {
			return nil, rerr
		}
	}
}

func (c *Conn) Close() error {
	return c.nc.Close()
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
