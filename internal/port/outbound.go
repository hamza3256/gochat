package port

import (
	"context"

	"github.com/hamza3256/gochat/internal/domain"
)

// PresenceStore tracks which node each user is connected to.
type PresenceStore interface {
	Register(ctx context.Context, sess *domain.Session) error
	Unregister(ctx context.Context, sess *domain.Session) error
	Lookup(ctx context.Context, userID string) (nodeID string, err error)
	Heartbeat(ctx context.Context, sess *domain.Session) error
}

// Publisher sends envelopes to remote gateway nodes.
type Publisher interface {
	Publish(ctx context.Context, targetNodeID string, env *domain.Envelope) error
	Subscribe(ctx context.Context, nodeID string, ch chan<- *domain.Envelope) error
	Close() error
}

// MessageStore provides durable append and acknowledgment of messages
// via the write-ahead log.
type MessageStore interface {
	Append(ctx context.Context, env *domain.Envelope) error
	Acknowledge(ctx context.Context, messageID string) error
	Recover(ctx context.Context, fn func(*domain.Envelope) error) error
	Close() error
}
