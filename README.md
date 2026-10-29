# GoChat

A single-process WebSocket messaging engine. v0 is a load-testable echo and small-room fan-out path: custom kqueue/epoll reactors, no Redis, no WAL, no JSON on the hot path.

Later work can add persistence and clustering. Those are not in this tree.

## Quick start

```bash
make build
make build-loadtest

./bin/gochat -listen :8080 -internal :6060

# another terminal
./bin/loadtest -addr 127.0.0.1:8080 -connections 1000 -duration 5s -mode echo
```

`make test` runs unit tests and a localhost echo/room integration test.

## What v0 measures

The load tester stamps unix-nano in the payload, waits for that stamp to come back, and prints p50/p90/p99/p999 in microseconds. A single connection on loopback is the latency number (tens of microseconds on a Mac). Hundreds of client goroutines will show scheduler delay, not engine delay.

| Mode | Flag | Behavior |
|------|------|----------|
| Echo | `-mode echo` | Server writes the same envelope back on the same connection |
| Pub | `-mode pub` | Connection joins `id/room-size`; publications go to every member **including the sender** so RTT is measurable |

Raise `ulimit -n` before large connection counts. `-max-conns` is the fd-indexed slab size and must exceed the highest file descriptor the process will see, not merely the connection count.

Health, JSON stats, and pprof listen on `-internal` (default `:6060`).

## Wire format

WebSocket binary frames, 5-byte envelope then opaque bytes:

| Offset | Type | Field |
|--------|------|--------|
| 0 | u8 | `1` echo, `2` join, `3` pub |
| 1–4 | u32 LE | room id |
| 5+ | bytes | payload (load tester: u64 LE timestamp + padding) |

IPv4 only. Unix (macOS kqueue / Linux epoll) only. Go 1.27.3.

## Layout

```
cmd/gochat/        server
cmd/loadtest/      latency client
internal/engine/   poller, reactor, rooms, connection slab
internal/ws/       RFC 6455 codec and upgrade
internal/wsclient/ stdlib test/load client
pkg/wire/          application envelope
pkg/bytebuf/       tiered buffer pool
pkg/hist/          percentile helper
```
