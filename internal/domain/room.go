package domain

type Room struct {
	ID      string
	Members map[string]struct{} // user IDs
}

func NewRoom(id string) *Room {
	return &Room{
		ID:      id,
		Members: make(map[string]struct{}),
	}
}

func (r *Room) AddMember(userID string) {
	r.Members[userID] = struct{}{}
}

func (r *Room) RemoveMember(userID string) {
	delete(r.Members, userID)
}

func (r *Room) HasMember(userID string) bool {
	_, ok := r.Members[userID]
	return ok
}

func (r *Room) MemberCount() int {
	return len(r.Members)
}
