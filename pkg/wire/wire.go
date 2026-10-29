// Package wire is the application envelope inside a WebSocket binary payload.
package wire

import (
	"encoding/binary"
	"errors"
)

const (
	OpEcho uint8 = 1
	OpJoin uint8 = 2
	OpPub  uint8 = 3

	HeaderSize = 5
)

var ErrShort = errors.New("wire: short envelope")

// Append writes op, room (little-endian u32), and payload onto dst.
func Append(dst []byte, op uint8, room uint32, payload []byte) []byte {
	dst = append(dst, op)
	dst = binary.LittleEndian.AppendUint32(dst, room)
	dst = append(dst, payload...)
	return dst
}

// EncodedLen is the envelope size for a payload of n bytes.
func EncodedLen(n int) int {
	return HeaderSize + n
}

// Parse splits an envelope. payload shares b's backing array.
func Parse(b []byte) (op uint8, room uint32, payload []byte, err error) {
	if len(b) < HeaderSize {
		return 0, 0, nil, ErrShort
	}
	return b[0], binary.LittleEndian.Uint32(b[1:5]), b[5:], nil
}
