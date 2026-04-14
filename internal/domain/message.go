package domain

import "time"

type DeliveryState uint8

const (
	StatePending      DeliveryState = iota
	StateDelivered
	StateAcknowledged
)

func (s DeliveryState) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateDelivered:
		return "delivered"
	case StateAcknowledged:
		return "acknowledged"
	default:
		return "unknown"
	}
}

type Message struct {
	ID        string        `msgpack:"id"`
	RoomID    string        `msgpack:"room_id"`
	SenderID  string        `msgpack:"sender_id"`
	Body      []byte        `msgpack:"body"`
	CreatedAt time.Time     `msgpack:"created_at"`
	State     DeliveryState `msgpack:"state"`
}

// Envelope wraps a message with routing metadata used on the wire and
// inside the ring buffer dispatcher.
type Envelope struct {
	Type      EnvelopeType `msgpack:"type"`
	MessageID string       `msgpack:"msg_id,omitempty"`
	RoomID    string       `msgpack:"room_id,omitempty"`
	SenderID  string       `msgpack:"sender_id,omitempty"`
	Body      []byte       `msgpack:"body,omitempty"`
	Timestamp int64        `msgpack:"ts"`
}

type EnvelopeType uint8

const (
	EnvelopeMessage  EnvelopeType = 1
	EnvelopeAck      EnvelopeType = 2
	EnvelopeJoin     EnvelopeType = 3
	EnvelopeLeave    EnvelopeType = 4
	EnvelopePing     EnvelopeType = 5
	EnvelopePong     EnvelopeType = 6
)
