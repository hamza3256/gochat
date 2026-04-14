package protocol

import (
	"testing"
	"time"

	"github.com/hamza3256/gochat/internal/domain"
	"github.com/hamza3256/gochat/pkg/bytebuf"
)

func makeEnvelope() *domain.Envelope {
	return &domain.Envelope{
		Type:      domain.EnvelopeMessage,
		MessageID: "msg-001",
		RoomID:    "room-42",
		SenderID:  "user-7",
		Body:      []byte("hello world"),
		Timestamp: time.Now().UnixNano(),
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	original := makeEnvelope()

	encoded, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	defer bytebuf.Put(encoded)

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type = %d, want %d", decoded.Type, original.Type)
	}
	if decoded.MessageID != original.MessageID {
		t.Errorf("MessageID = %q, want %q", decoded.MessageID, original.MessageID)
	}
	if decoded.RoomID != original.RoomID {
		t.Errorf("RoomID = %q, want %q", decoded.RoomID, original.RoomID)
	}
	if decoded.SenderID != original.SenderID {
		t.Errorf("SenderID = %q, want %q", decoded.SenderID, original.SenderID)
	}
	if string(decoded.Body) != string(original.Body) {
		t.Errorf("Body = %q, want %q", decoded.Body, original.Body)
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %d, want %d", decoded.Timestamp, original.Timestamp)
	}
}

func TestDecodeInto(t *testing.T) {
	original := makeEnvelope()
	encoded, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	defer bytebuf.Put(encoded)

	var env domain.Envelope
	if err := DecodeInto(encoded, &env); err != nil {
		t.Fatalf("DecodeInto: %v", err)
	}
	if env.MessageID != original.MessageID {
		t.Errorf("MessageID = %q, want %q", env.MessageID, original.MessageID)
	}
}

func TestEncodeTo(t *testing.T) {
	original := makeEnvelope()
	dst := make([]byte, 512)

	n, err := EncodeTo(dst, original)
	if err != nil {
		t.Fatalf("EncodeTo: %v", err)
	}
	if n == 0 {
		t.Fatal("EncodeTo wrote 0 bytes")
	}

	decoded, err := Decode(dst[:n])
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if decoded.MessageID != original.MessageID {
		t.Errorf("MessageID = %q, want %q", decoded.MessageID, original.MessageID)
	}
}

func BenchmarkEncode(b *testing.B) {
	env := makeEnvelope()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _ := Encode(env)
		bytebuf.Put(buf)
	}
}

func BenchmarkDecode(b *testing.B) {
	env := makeEnvelope()
	encoded, _ := Encode(env)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(encoded)
	}
	bytebuf.Put(encoded)
}

func BenchmarkDecodeInto(b *testing.B) {
	original := makeEnvelope()
	encoded, _ := Encode(original)
	var env domain.Envelope
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodeInto(encoded, &env)
	}
	bytebuf.Put(encoded)
}
