package engine

import "testing"

func TestRoomsJoinLeaveAndIsolation(t *testing.T) {
	r := NewRooms()
	r.Join(1, Member{Fd: 10, ReactorID: 0})
	r.Join(1, Member{Fd: 11, ReactorID: 0})
	r.Join(2, Member{Fd: 20, ReactorID: 1})

	m1 := r.Members(1)
	if len(m1) != 2 {
		t.Fatalf("room 1 size=%d, want 2", len(m1))
	}
	m2 := r.Members(2)
	if len(m2) != 1 || m2[0].Fd != 20 {
		t.Fatalf("room 2 = %+v", m2)
	}

	r.Leave(10)
	m1 = r.Members(1)
	if len(m1) != 1 || m1[0].Fd != 11 {
		t.Fatalf("after leave room 1 = %+v", m1)
	}
	if len(r.Members(2)) != 1 {
		t.Fatal("room 2 should be untouched")
	}

	r.Join(2, Member{Fd: 11, ReactorID: 0})
	if len(r.Members(1)) != 0 {
		t.Fatal("fd 11 should have left room 1 on re-join")
	}
	if len(r.Members(2)) != 2 {
		t.Fatalf("room 2 size=%d, want 2", len(r.Members(2)))
	}
}
