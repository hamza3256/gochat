package wire

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestAppendParseRoundTrip(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	raw := Append(nil, OpPub, 0x01020304, payload)

	if len(raw) != EncodedLen(len(payload)) {
		t.Fatalf("len=%d, want %d", len(raw), EncodedLen(len(payload)))
	}
	if raw[0] != OpPub {
		t.Fatalf("op byte = %d", raw[0])
	}
	if got := binary.LittleEndian.Uint32(raw[1:5]); got != 0x01020304 {
		t.Fatalf("room = %#x, want 0x01020304", got)
	}

	op, room, gotPayload, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if op != OpPub || room != 0x01020304 {
		t.Fatalf("op=%d room=%#x", op, room)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("payload=%x, want %x", gotPayload, payload)
	}
}

func TestParseShort(t *testing.T) {
	if _, _, _, err := Parse([]byte{1, 2, 3, 4}); err != ErrShort {
		t.Fatalf("err=%v, want ErrShort", err)
	}
}

func TestParseEmptyPayload(t *testing.T) {
	raw := Append(nil, OpJoin, 7, nil)
	op, room, payload, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if op != OpJoin || room != 7 || len(payload) != 0 {
		t.Fatalf("op=%d room=%d payload=%d", op, room, len(payload))
	}
}
