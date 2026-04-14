package service

import (
	"context"
	"log"
	"time"

	"github.com/hamza3256/gochat/internal/domain"
	"github.com/hamza3256/gochat/internal/port"
)

// PresenceService manages user heartbeats and subscribes to remote
// envelope delivery from other nodes via Pub/Sub.
type PresenceService struct {
	nodeID   string
	store    port.PresenceStore
	pub      port.Publisher
	remoteCh chan *domain.Envelope
	onRemote func(*domain.Envelope)
}

func NewPresenceService(
	nodeID string,
	store port.PresenceStore,
	pub port.Publisher,
	onRemote func(*domain.Envelope),
) *PresenceService {
	return &PresenceService{
		nodeID:   nodeID,
		store:    store,
		pub:      pub,
		remoteCh: make(chan *domain.Envelope, 4096),
		onRemote: onRemote,
	}
}

// Start begins listening for cross-node messages and runs until ctx
// is cancelled.
func (ps *PresenceService) Start(ctx context.Context) error {
	if ps.pub == nil {
		return nil
	}

	if err := ps.pub.Subscribe(ctx, ps.nodeID, ps.remoteCh); err != nil {
		return err
	}

	go ps.consumeRemote(ctx)
	return nil
}

func (ps *PresenceService) consumeRemote(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-ps.remoteCh:
			if !ok {
				return
			}
			if ps.onRemote != nil {
				ps.onRemote(env)
			}
		}
	}
}

// RunHeartbeat periodically refreshes the presence TTL for a session.
func (ps *PresenceService) RunHeartbeat(ctx context.Context, sess *domain.Session, interval time.Duration) {
	if ps.store == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ps.store.Heartbeat(ctx, sess); err != nil {
				log.Printf("presence heartbeat error user=%s: %v", sess.UserID, err)
			}
		}
	}
}
