package ws

import (
	"bytes"
	"io"
	"testing"
)

func TestEncodeDecodeUnmasked(t *testing.T) {
	payload := []byte("hello")
	buf := make([]byte, FrameOverhead(len(payload)))
	n, err := EncodeFrame(buf, OpBinary, payload)
	if err != nil {
		t.Fatal(err)
	}
	frame, consumed, err := DecodeFrame(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	if consumed != n || frame.Opcode != OpBinary || !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("frame=%+v consumed=%d", frame, consumed)
	}
}

func TestEncodeDecodeMasked(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5, 6, 7}
	buf := make([]byte, FrameOverheadMasked(len(payload)))
	n, err := EncodeFrameMasked(buf, OpBinary, payload)
	if err != nil {
		t.Fatal(err)
	}
	frame, _, err := DecodeFrame(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	if !frame.Masked || !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("payload=%v masked=%v", frame.Payload, frame.Masked)
	}
}

func TestDecodeShort(t *testing.T) {
	_, _, err := DecodeFrame([]byte{0x82})
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("err=%v", err)
	}
}
