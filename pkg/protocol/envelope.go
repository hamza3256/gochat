package protocol

import (
	"errors"

	"github.com/hamza3256/gochat/internal/domain"
	"github.com/hamza3256/gochat/pkg/bytebuf"
	"github.com/vmihailenco/msgpack/v5"
)

// Encode serializes an Envelope into a pooled byte slice.
// The caller must call bytebuf.Put on the returned slice when done.
func Encode(env *domain.Envelope) ([]byte, error) {
	data, err := msgpack.Marshal(env)
	if err != nil {
		return nil, err
	}
	buf := bytebuf.Get(len(data))
	copy(buf, data)
	return buf, nil
}

// EncodeTo serializes an Envelope directly into the provided buffer,
// returning the number of bytes written. If dst is too small the
// function falls back to Encode and copies into a new pooled buffer.
func EncodeTo(dst []byte, env *domain.Envelope) (int, error) {
	data, err := msgpack.Marshal(env)
	if err != nil {
		return 0, err
	}
	if len(dst) < len(data) {
		return 0, ErrBufferTooSmall
	}
	n := copy(dst, data)
	return n, nil
}

// Decode deserializes an Envelope from raw bytes without copying the
// input slice (msgpack decodes in place where possible).
func Decode(data []byte) (*domain.Envelope, error) {
	env := &domain.Envelope{}
	if err := msgpack.Unmarshal(data, env); err != nil {
		return nil, err
	}
	return env, nil
}

// DecodeInto unmarshals into a caller-provided Envelope to avoid
// allocation of the envelope struct itself.
func DecodeInto(data []byte, env *domain.Envelope) error {
	return msgpack.Unmarshal(data, env)
}

var ErrBufferTooSmall = errors.New("protocol: destination buffer too small")
