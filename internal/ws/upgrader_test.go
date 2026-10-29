package ws

import "testing"

func TestAcceptKeyRFC6455(t *testing.T) {
	// RFC 6455 section 1.3 / 4.2.2 example.
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	const want = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := AcceptKey(key); got != want {
		t.Fatalf("AcceptKey = %q, want %q", got, want)
	}
}

func TestParseUpgradeRequest(t *testing.T) {
	req := "GET /ws HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\nextra"
	got, n, err := ParseUpgradeRequest([]byte(req))
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "dGhlIHNhbXBsZSBub25jZQ==" || got.Path != "/ws" {
		t.Fatalf("%+v", got)
	}
	if n != len(req)-len("extra") {
		t.Fatalf("consumed %d, want %d", n, len(req)-len("extra"))
	}
}

func TestParseUpgradeIncomplete(t *testing.T) {
	_, _, err := ParseUpgradeRequest([]byte("GET /ws HTTP/1.1\r\n"))
	if err != ErrIncomplete {
		t.Fatalf("err=%v, want ErrIncomplete", err)
	}
}
