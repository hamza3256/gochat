package presence

import (
	"context"
	"fmt"
	"time"

	"github.com/hamza3256/gochat/internal/domain"
	"github.com/hamza3256/gochat/pkg/protocol"
	"github.com/redis/go-redis/v9"
)

const (
	presenceKeyPrefix = "gochat:presence:"
	nodeChannelPrefix = "gochat:node:"
)

// RedisPresenceConfig holds Redis-specific tuning knobs.
type RedisPresenceConfig struct {
	TTL               time.Duration
	HeartbeatInterval time.Duration
}

// RedisPresence implements port.PresenceStore and port.Publisher using
// Redis hashes for user-to-node mapping and Pub/Sub for cross-node
// message routing.
type RedisPresence struct {
	client *redis.Client
	cfg    RedisPresenceConfig
}

func NewRedisPresence(client *redis.Client, cfg RedisPresenceConfig) *RedisPresence {
	return &RedisPresence{client: client, cfg: cfg}
}

func presenceKey(userID string) string {
	return presenceKeyPrefix + userID
}

func nodeChannel(nodeID string) string {
	return nodeChannelPrefix + nodeID
}

// --- PresenceStore implementation ---

func (rp *RedisPresence) Register(ctx context.Context, sess *domain.Session) error {
	key := presenceKey(sess.UserID)
	pipe := rp.client.Pipeline()
	pipe.HSet(ctx, key,
		"node_id", sess.NodeID,
		"connected_at", sess.ConnectedAt.Unix(),
	)
	pipe.Expire(ctx, key, rp.cfg.TTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (rp *RedisPresence) Unregister(ctx context.Context, sess *domain.Session) error {
	return rp.client.Del(ctx, presenceKey(sess.UserID)).Err()
}

func (rp *RedisPresence) Lookup(ctx context.Context, userID string) (string, error) {
	nodeID, err := rp.client.HGet(ctx, presenceKey(userID), "node_id").Result()
	if err == redis.Nil {
		return "", nil
	}
	return nodeID, err
}

func (rp *RedisPresence) Heartbeat(ctx context.Context, sess *domain.Session) error {
	return rp.client.Expire(ctx, presenceKey(sess.UserID), rp.cfg.TTL).Err()
}

// --- Publisher implementation ---

func (rp *RedisPresence) Publish(ctx context.Context, targetNodeID string, env *domain.Envelope) error {
	data, err := protocol.Encode(env)
	if err != nil {
		return fmt.Errorf("presence publish encode: %w", err)
	}
	return rp.client.Publish(ctx, nodeChannel(targetNodeID), data).Err()
}

func (rp *RedisPresence) Subscribe(ctx context.Context, nodeID string, ch chan<- *domain.Envelope) error {
	pubsub := rp.client.Subscribe(ctx, nodeChannel(nodeID))

	go func() {
		defer pubsub.Close()
		msgCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				env, err := protocol.Decode([]byte(msg.Payload))
				if err != nil {
					continue
				}
				select {
				case ch <- env:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return nil
}

func (rp *RedisPresence) Close() error {
	return rp.client.Close()
}
