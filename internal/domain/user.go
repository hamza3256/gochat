package domain

import "time"

type PresenceStatus uint8

const (
	PresenceOnline  PresenceStatus = iota
	PresenceAway
	PresenceOffline
)

type User struct {
	ID       string
	Name     string
	Status   PresenceStatus
}

type Session struct {
	UserID      string
	NodeID      string
	ConnFD      int
	ConnectedAt time.Time
}
