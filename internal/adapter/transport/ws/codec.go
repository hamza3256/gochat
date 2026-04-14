package ws

import (
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
)

// WebSocket opcodes per RFC 6455.
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

var (
	ErrFrameTooLarge = errors.New("ws: frame exceeds max size")
	ErrInvalidFrame  = errors.New("ws: invalid frame")
	ErrShortBuffer   = errors.New("ws: buffer too short for frame")
)

// Frame represents a decoded WebSocket frame.
type Frame struct {
	Fin     bool
	Opcode  byte
	Masked  bool
	Payload []byte
}

const maxFrameSize = 1 << 20 // 1 MB

// DecodeFrame reads a single WebSocket frame from buf, returning the
// frame and the number of bytes consumed. If buf is too short it
// returns io.ErrUnexpectedEOF so the caller knows to buffer more data.
func DecodeFrame(buf []byte) (Frame, int, error) {
	if len(buf) < 2 {
		return Frame{}, 0, io.ErrUnexpectedEOF
	}

	pos := 0
	b0 := buf[pos]
	pos++
	b1 := buf[pos]
	pos++

	fin := b0&0x80 != 0
	opcode := b0 & 0x0F
	masked := b1&0x80 != 0
	length := uint64(b1 & 0x7F)

	switch {
	case length == 126:
		if len(buf) < pos+2 {
			return Frame{}, 0, io.ErrUnexpectedEOF
		}
		length = uint64(binary.BigEndian.Uint16(buf[pos : pos+2]))
		pos += 2
	case length == 127:
		if len(buf) < pos+8 {
			return Frame{}, 0, io.ErrUnexpectedEOF
		}
		length = binary.BigEndian.Uint64(buf[pos : pos+8])
		pos += 8
	}

	if length > maxFrameSize {
		return Frame{}, 0, ErrFrameTooLarge
	}

	var mask [4]byte
	if masked {
		if len(buf) < pos+4 {
			return Frame{}, 0, io.ErrUnexpectedEOF
		}
		copy(mask[:], buf[pos:pos+4])
		pos += 4
	}

	total := pos + int(length)
	if len(buf) < total {
		return Frame{}, 0, io.ErrUnexpectedEOF
	}

	payload := buf[pos:total]
	if masked {
		maskBytes(payload, mask)
	}

	return Frame{
		Fin:     fin,
		Opcode:  opcode,
		Masked:  masked,
		Payload: payload,
	}, total, nil
}

// EncodeFrame writes a WebSocket frame into dst. Returns the number
// of bytes written. The frame is NOT masked (server-to-client).
func EncodeFrame(dst []byte, opcode byte, payload []byte) (int, error) {
	needed := FrameOverhead(len(payload))
	if len(dst) < needed {
		return 0, ErrShortBuffer
	}

	pos := 0
	dst[pos] = 0x80 | (opcode & 0x0F) // FIN + opcode
	pos++

	plen := len(payload)
	switch {
	case plen <= 125:
		dst[pos] = byte(plen)
		pos++
	case plen <= 65535:
		dst[pos] = 126
		pos++
		binary.BigEndian.PutUint16(dst[pos:], uint16(plen))
		pos += 2
	default:
		dst[pos] = 127
		pos++
		binary.BigEndian.PutUint64(dst[pos:], uint64(plen))
		pos += 8
	}

	copy(dst[pos:], payload)
	pos += plen
	return pos, nil
}

// FrameOverhead returns the number of header bytes needed for a
// payload of the given size (unmasked, server-to-client).
func FrameOverhead(payloadLen int) int {
	switch {
	case payloadLen <= 125:
		return 2 + payloadLen
	case payloadLen <= 65535:
		return 4 + payloadLen
	default:
		return 10 + payloadLen
	}
}

// MakeMaskKey generates a random 4-byte mask key.
func MakeMaskKey() [4]byte {
	var k [4]byte
	v := rand.Uint32()
	binary.LittleEndian.PutUint32(k[:], v)
	return k
}

// EncodeFrameMasked writes a masked frame (client-to-server).
func EncodeFrameMasked(dst []byte, opcode byte, payload []byte) (int, error) {
	maskKey := MakeMaskKey()
	needed := FrameOverheadMasked(len(payload))
	if len(dst) < needed {
		return 0, ErrShortBuffer
	}

	pos := 0
	dst[pos] = 0x80 | (opcode & 0x0F)
	pos++

	plen := len(payload)
	switch {
	case plen <= 125:
		dst[pos] = 0x80 | byte(plen)
		pos++
	case plen <= 65535:
		dst[pos] = 0x80 | 126
		pos++
		binary.BigEndian.PutUint16(dst[pos:], uint16(plen))
		pos += 2
	default:
		dst[pos] = 0x80 | 127
		pos++
		binary.BigEndian.PutUint64(dst[pos:], uint64(plen))
		pos += 8
	}

	copy(dst[pos:], maskKey[:])
	pos += 4

	copy(dst[pos:], payload)
	maskBytes(dst[pos:pos+plen], maskKey)
	pos += plen
	return pos, nil
}

func FrameOverheadMasked(payloadLen int) int {
	return FrameOverhead(payloadLen) + 4
}

func maskBytes(b []byte, mask [4]byte) {
	for i := range b {
		b[i] ^= mask[i%4]
	}
}
