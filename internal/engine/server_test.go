//go:build darwin || linux

package engine_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/hamza3256/gochat/internal/engine"
	"github.com/hamza3256/gochat/internal/wsclient"
	"github.com/hamza3256/gochat/pkg/wire"
)

func startTestServer(t *testing.T, reactors int) *engine.Server {
	t.Helper()
	s, err := engine.NewServer(engine.Config{
		Addr:        "127.0.0.1:0",
		NumReactors: reactors,
		MaxConns:    1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	t.Cleanup(s.Close)
	return s
}

func dial(t *testing.T, addr string) *wsclient.Conn {
	t.Helper()
	c, err := wsclient.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEchoRoundTrip(t *testing.T) {
	s := startTestServer(t, 1)
	c := dial(t, s.Addr())

	ts := uint64(time.Now().UnixNano())
	payload := make([]byte, 8)
	binary.LittleEndian.PutUint64(payload, ts)
	env := wire.Append(nil, wire.OpEcho, 0, payload)

	if err := c.WriteBinary(env); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, env) {
		t.Fatalf("echo mismatch\n got %x\nwant %x", got, env)
	}
}

func TestPubStaysInRoom(t *testing.T) {
	s := startTestServer(t, 1)
	a := dial(t, s.Addr())
	b := dial(t, s.Addr())
	outsider := dial(t, s.Addr())

	if err := a.WriteBinary(wire.Append(nil, wire.OpJoin, 1, nil)); err != nil {
		t.Fatal(err)
	}
	if err := b.WriteBinary(wire.Append(nil, wire.OpJoin, 1, nil)); err != nil {
		t.Fatal(err)
	}
	if err := outsider.WriteBinary(wire.Append(nil, wire.OpJoin, 2, nil)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.Snapshot().Joins >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if s.Snapshot().Joins < 3 {
		t.Fatalf("joins=%d, want 3", s.Snapshot().Joins)
	}

	body := []byte("hello-room-1")
	msg := wire.Append(nil, wire.OpPub, 1, body)
	if err := a.WriteBinary(msg); err != nil {
		t.Fatal(err)
	}

	gotA, err := a.ReadBinary()
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := b.ReadBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotA, msg) || !bytes.Equal(gotB, msg) {
		t.Fatalf("members did not receive pub")
	}

	_ = outsider.SetDeadline(time.Now().Add(150 * time.Millisecond))
	if _, err := outsider.ReadBinary(); err == nil {
		t.Fatal("room 2 should not receive room 1 publication")
	}
}

func TestEchoTwoReactors(t *testing.T) {
	s := startTestServer(t, 2)
	c := dial(t, s.Addr())
	env := wire.Append(nil, wire.OpEcho, 0, []byte("x"))
	if err := c.WriteBinary(env); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, env) {
		t.Fatalf("got %x want %x", got, env)
	}
}
