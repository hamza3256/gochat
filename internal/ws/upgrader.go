package ws

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"strings"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

var (
	ErrIncomplete   = errors.New("ws: incomplete HTTP upgrade")
	ErrNotWebSocket = errors.New("ws: not a WebSocket upgrade")
	ErrBadRequest   = errors.New("ws: bad HTTP upgrade request")
)

type UpgradeRequest struct {
	Path    string
	Key     string
	Version string
}

func ParseUpgradeRequest(buf []byte) (UpgradeRequest, int, error) {
	s := string(buf)
	end := strings.Index(s, "\r\n\r\n")
	if end < 0 {
		return UpgradeRequest{}, 0, ErrIncomplete
	}
	consumed := end + 4

	lines := strings.Split(s[:end], "\r\n")
	if len(lines) < 1 {
		return UpgradeRequest{}, 0, ErrBadRequest
	}

	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) < 3 || parts[0] != "GET" {
		return UpgradeRequest{}, 0, ErrBadRequest
	}

	req := UpgradeRequest{Path: parts[1]}
	hasUpgrade := false

	for _, line := range lines[1:] {
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		switch strings.ToLower(key) {
		case "upgrade":
			if strings.EqualFold(val, "websocket") {
				hasUpgrade = true
			}
		case "sec-websocket-key":
			req.Key = val
		case "sec-websocket-version":
			req.Version = val
		}
	}

	if !hasUpgrade || req.Key == "" {
		return UpgradeRequest{}, 0, ErrNotWebSocket
	}
	return req, consumed, nil
}

func BuildUpgradeResponse(key string) []byte {
	accept := AcceptKey(key)
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	return []byte(resp)
}

func AcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
