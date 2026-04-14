package ws

import (
	"sync"

	"github.com/hamza3256/gochat/pkg/bytebuf"
)

// WSConn is a lightweight WebSocket connection state machine that
// layers on top of the raw fd managed by the engine. It handles
// frame reassembly and upgrade state.
type WSConn struct {
	Upgraded bool
	recvBuf  []byte // partial data waiting for a complete frame
	mu       sync.Mutex
}

// Feed appends raw bytes and attempts to decode complete frames.
// Returns decoded payload frames and any upgrade request encountered.
func (wc *WSConn) Feed(data []byte) (payloads [][]byte, upgrade *UpgradeRequest, err error) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	wc.recvBuf = append(wc.recvBuf, data...)

	if !wc.Upgraded {
		req, consumed, parseErr := ParseUpgradeRequest(wc.recvBuf)
		if parseErr != nil {
			if parseErr == ErrBadRequest {
				return nil, nil, nil // need more data
			}
			return nil, nil, parseErr
		}
		wc.recvBuf = wc.recvBuf[consumed:]
		wc.Upgraded = true
		return nil, &req, nil
	}

	for len(wc.recvBuf) > 0 {
		frame, n, frameErr := DecodeFrame(wc.recvBuf)
		if frameErr != nil {
			break // need more data
		}
		wc.recvBuf = wc.recvBuf[n:]

		switch frame.Opcode {
		case OpText, OpBinary:
			payload := bytebuf.Get(len(frame.Payload))
			copy(payload, frame.Payload)
			payloads = append(payloads, payload)
		case OpPing:
			// Pong is handled at the transport layer
			payloads = append(payloads, nil) // signal ping
		case OpClose:
			return payloads, nil, ErrClosed
		}
	}
	return payloads, nil, nil
}

// Reset clears the connection state for reuse.
func (wc *WSConn) Reset() {
	wc.mu.Lock()
	wc.Upgraded = false
	wc.recvBuf = wc.recvBuf[:0]
	wc.mu.Unlock()
}

var ErrClosed = errClosed{}

type errClosed struct{}

func (errClosed) Error() string { return "ws: connection closed by peer" }
