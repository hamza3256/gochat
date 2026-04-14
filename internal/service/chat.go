package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/hamza3256/gochat/internal/adapter/transport/ws"
	"github.com/hamza3256/gochat/internal/domain"
	"github.com/hamza3256/gochat/internal/engine"
	"github.com/hamza3256/gochat/internal/port"
	"github.com/hamza3256/gochat/pkg/bytebuf"
	"github.com/hamza3256/gochat/pkg/protocol"
)

// ChatService orchestrates message routing, local fan-out, remote
// delivery, and WAL persistence. It implements engine.MessageSink
// so the dispatcher can feed raw data directly into it.
type ChatService struct {
	nodeID   string
	presence port.PresenceStore
	pub      port.Publisher
	store    port.MessageStore
	pool     *engine.ConnPool
	reactors []*engine.Reactor

	rooms   map[string]*domain.Room
	roomsMu sync.RWMutex

	// fd -> WSConn state machine
	wsConns   map[int]*ws.WSConn
	wsConnsMu sync.RWMutex

	// fd -> session
	sessions   map[int]*domain.Session
	sessionsMu sync.RWMutex
}

func NewChatService(
	nodeID string,
	presence port.PresenceStore,
	pub port.Publisher,
	store port.MessageStore,
	pool *engine.ConnPool,
	reactors []*engine.Reactor,
) *ChatService {
	return &ChatService{
		nodeID:   nodeID,
		presence: presence,
		pub:      pub,
		store:    store,
		pool:     pool,
		reactors: reactors,
		rooms:    make(map[string]*domain.Room),
		wsConns:  make(map[int]*ws.WSConn),
		sessions: make(map[int]*domain.Session),
	}
}

// --- engine.MessageSink implementation ---

func (cs *ChatService) OnData(fd int, data []byte) error {
	wsc := cs.getOrCreateWSConn(fd)

	payloads, upgrade, err := wsc.Feed(data)
	if err != nil {
		return err
	}

	if upgrade != nil {
		resp := ws.BuildUpgradeResponse(upgrade.Key)
		cs.writeToFd(fd, resp)
		return nil
	}

	for _, payload := range payloads {
		if payload == nil {
			cs.sendPong(fd)
			continue
		}
		cs.handlePayload(fd, payload)
		bytebuf.Put(payload)
	}
	return nil
}

func (cs *ChatService) OnDisconnect(fd int) {
	cs.wsConnsMu.Lock()
	delete(cs.wsConns, fd)
	cs.wsConnsMu.Unlock()

	cs.sessionsMu.Lock()
	sess, ok := cs.sessions[fd]
	delete(cs.sessions, fd)
	cs.sessionsMu.Unlock()

	if ok && sess != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cs.presence.Unregister(ctx, sess)
		cs.removeFromAllRooms(sess.UserID)
	}
}

func (cs *ChatService) handlePayload(fd int, payload []byte) {
	env, err := protocol.Decode(payload)
	if err != nil {
		log.Printf("chat: decode error fd=%d: %v", fd, err)
		return
	}

	switch env.Type {
	case domain.EnvelopeMessage:
		cs.handleMessage(fd, env)
	case domain.EnvelopeAck:
		cs.handleAck(env)
	case domain.EnvelopeJoin:
		cs.handleJoin(fd, env)
	case domain.EnvelopeLeave:
		cs.handleLeave(fd, env)
	case domain.EnvelopePing:
		cs.sendPong(fd)
	}
}

func (cs *ChatService) handleMessage(fd int, env *domain.Envelope) {
	if cs.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cs.store.Append(ctx, env)
	}

	cs.roomsMu.RLock()
	room, ok := cs.rooms[env.RoomID]
	cs.roomsMu.RUnlock()
	if !ok {
		return
	}

	for memberID := range room.Members {
		if memberID == env.SenderID {
			continue
		}
		cs.deliverToUser(memberID, env)
	}
}

func (cs *ChatService) handleAck(env *domain.Envelope) {
	if cs.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cs.store.Acknowledge(ctx, env.MessageID)
	}
}

func (cs *ChatService) handleJoin(fd int, env *domain.Envelope) {
	sess := &domain.Session{
		UserID:      env.SenderID,
		NodeID:      cs.nodeID,
		ConnFD:      fd,
		ConnectedAt: time.Now(),
	}

	cs.sessionsMu.Lock()
	cs.sessions[fd] = sess
	cs.sessionsMu.Unlock()

	if cs.presence != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cs.presence.Register(ctx, sess)
	}

	cs.roomsMu.Lock()
	room, ok := cs.rooms[env.RoomID]
	if !ok {
		room = domain.NewRoom(env.RoomID)
		cs.rooms[env.RoomID] = room
	}
	room.AddMember(env.SenderID)
	cs.roomsMu.Unlock()
}

func (cs *ChatService) handleLeave(fd int, env *domain.Envelope) {
	cs.roomsMu.Lock()
	if room, ok := cs.rooms[env.RoomID]; ok {
		room.RemoveMember(env.SenderID)
		if room.MemberCount() == 0 {
			delete(cs.rooms, env.RoomID)
		}
	}
	cs.roomsMu.Unlock()
}

func (cs *ChatService) deliverToUser(userID string, env *domain.Envelope) {
	fd := cs.findLocalFd(userID)
	if fd >= 0 {
		data, err := protocol.Encode(env)
		if err != nil {
			return
		}
		cs.sendWSFrame(fd, data)
		bytebuf.Put(data)
		return
	}

	if cs.presence == nil || cs.pub == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	nodeID, err := cs.presence.Lookup(ctx, userID)
	if err != nil || nodeID == "" || nodeID == cs.nodeID {
		return
	}
	cs.pub.Publish(ctx, nodeID, env)
}

func (cs *ChatService) findLocalFd(userID string) int {
	cs.sessionsMu.RLock()
	defer cs.sessionsMu.RUnlock()
	for fd, sess := range cs.sessions {
		if sess.UserID == userID {
			return fd
		}
	}
	return -1
}

func (cs *ChatService) removeFromAllRooms(userID string) {
	cs.roomsMu.Lock()
	defer cs.roomsMu.Unlock()
	for id, room := range cs.rooms {
		room.RemoveMember(userID)
		if room.MemberCount() == 0 {
			delete(cs.rooms, id)
		}
	}
}

func (cs *ChatService) sendPong(fd int) {
	pong := bytebuf.Get(2)
	pong[0] = 0x80 | ws.OpPong
	pong[1] = 0
	cs.writeToFd(fd, pong)
}

func (cs *ChatService) sendWSFrame(fd int, payload []byte) {
	frame := bytebuf.Get(ws.FrameOverhead(len(payload)))
	n, err := ws.EncodeFrame(frame, ws.OpBinary, payload)
	if err != nil {
		bytebuf.Put(frame)
		return
	}
	cs.writeToFd(fd, frame[:n])
}

func (cs *ChatService) writeToFd(fd int, data []byte) {
	for _, r := range cs.reactors {
		if err := r.WriteToFd(fd, data); err == nil {
			return
		}
	}
}

// HandleRemoteEnvelope processes an envelope received from another
// node via Redis Pub/Sub.
func (cs *ChatService) HandleRemoteEnvelope(env *domain.Envelope) {
	if env.Type != domain.EnvelopeMessage {
		return
	}

	cs.roomsMu.RLock()
	room, ok := cs.rooms[env.RoomID]
	cs.roomsMu.RUnlock()
	if !ok {
		return
	}

	for memberID := range room.Members {
		if memberID == env.SenderID {
			continue
		}
		fd := cs.findLocalFd(memberID)
		if fd >= 0 {
			data, err := protocol.Encode(env)
			if err != nil {
				continue
			}
			cs.sendWSFrame(fd, data)
			bytebuf.Put(data)
		}
	}
}

func (cs *ChatService) getOrCreateWSConn(fd int) *ws.WSConn {
	cs.wsConnsMu.RLock()
	wsc, ok := cs.wsConns[fd]
	cs.wsConnsMu.RUnlock()
	if ok {
		return wsc
	}

	cs.wsConnsMu.Lock()
	defer cs.wsConnsMu.Unlock()
	if wsc, ok = cs.wsConns[fd]; ok {
		return wsc
	}
	wsc = &ws.WSConn{}
	cs.wsConns[fd] = wsc
	return wsc
}
