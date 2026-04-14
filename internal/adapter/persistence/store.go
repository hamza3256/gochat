package persistence

import (
	"context"
	"fmt"

	"github.com/hamza3256/gochat/internal/domain"
	"github.com/hamza3256/gochat/pkg/protocol"
)

// WALStore implements port.MessageStore by writing envelopes to the WAL.
type WALStore struct {
	wal    *WAL
	reader *Reader
}

func NewWALStore(wal *WAL, reader *Reader) *WALStore {
	return &WALStore{wal: wal, reader: reader}
}

func (s *WALStore) Append(_ context.Context, env *domain.Envelope) error {
	data, err := protocol.Encode(env)
	if err != nil {
		return fmt.Errorf("wal store: encode: %w", err)
	}
	return s.wal.Append(RecordTypeMessage, data)
}

func (s *WALStore) Acknowledge(_ context.Context, messageID string) error {
	return s.wal.Append(RecordTypeAck, []byte(messageID))
}

func (s *WALStore) Recover(_ context.Context, fn func(*domain.Envelope) error) error {
	_, err := s.reader.Replay(func(recordType byte, payload []byte) error {
		if recordType != RecordTypeMessage {
			return nil
		}
		env, decErr := protocol.Decode(payload)
		if decErr != nil {
			return nil // skip malformed
		}
		return fn(env)
	})
	return err
}

func (s *WALStore) Close() error {
	return s.wal.Close()
}
