package engine

import "sync"

type Member struct {
	Fd        int
	ReactorID int
}

type Rooms struct {
	mu    sync.RWMutex
	rooms map[uint32]*roomState
}

type roomState struct {
	members []Member
}

func NewRooms() *Rooms {
	return &Rooms{rooms: make(map[uint32]*roomState)}
}

func (r *Rooms) Join(id uint32, m Member) {
	r.Leave(m.Fd)
	r.mu.Lock()
	defer r.mu.Unlock()
	rs := r.rooms[id]
	if rs == nil {
		rs = &roomState{}
		r.rooms[id] = rs
	}
	rs.members = append(rs.members, m)
}

func (r *Rooms) Leave(fd int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, rs := range r.rooms {
		n := 0
		for _, m := range rs.members {
			if m.Fd != fd {
				rs.members[n] = m
				n++
			}
		}
		if n == 0 {
			delete(r.rooms, id)
			continue
		}
		rs.members = rs.members[:n]
	}
}

func (r *Rooms) Members(id uint32) []Member {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rs := r.rooms[id]
	if rs == nil {
		return nil
	}
	out := make([]Member, len(rs.members))
	copy(out, rs.members)
	return out
}
