package port

import "github.com/hamza3256/gochat/internal/domain"

// ConnectionHandler is the inbound port called by the transport layer
// when connection lifecycle events occur.
type ConnectionHandler interface {
	OnConnect(sess *domain.Session) error
	OnDisconnect(sess *domain.Session)
}

// MessageHandler is the inbound port called by the dispatcher when a
// decoded envelope is ready for processing.
type MessageHandler interface {
	HandleMessage(env *domain.Envelope, sess *domain.Session) error
}
