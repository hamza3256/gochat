# GoChat

A distributed messaging engine designed to sustain 1 million concurrent
WebSocket connections with p99 end-to-end latency below 50 ms.

## Architecture

GoChat follows hexagonal (ports-and-adapters) architecture:

- **Engine core** — custom epoll/kqueue poller, lock-free MPMC ring buffer,
  slab-allocated connection pool.
- **Transport adapter** — zero-copy WebSocket codec with sync.Pool buffer
  recycling.
- **Presence adapter** — Redis-backed user-to-node mapping with Pub/Sub for
  cross-node message routing.
- **Persistence adapter** — Write-Ahead Log with CRC32 integrity, segment
  rotation, and batch fdatasync.
- **Observability** — Prometheus metrics, pprof endpoints, HDR histogram
  tail-latency tracking.

## Quick Start

```bash
# Build the server
make build

# Build the load-testing tool
make build-loadtest

# Run tests
make test

# Run benchmarks
make bench

# Start the server
./bin/gochat -config configs/gochat.yaml
```

## Project Layout

```
cmd/gochat/        Gateway server entry point
cmd/loadtest/      1M-connection benchmarking tool
internal/domain/   Core business entities
internal/port/     Inbound and outbound port interfaces
internal/service/  Application services (ChatService, PresenceService)
internal/adapter/  Transport, presence, and persistence adapters
internal/engine/   Poller, reactor, ring buffer, dispatcher
pkg/bytebuf/      Tiered sync.Pool byte-buffer recycling
pkg/protocol/     msgpack wire-protocol codec
configs/           YAML configuration
```

## Configuration

See `configs/gochat.yaml` for all tunables. Key settings:

| Setting | Default | Description |
|---------|---------|-------------|
| `server.max_connections` | 1,048,576 | Pre-allocated connection slots |
| `engine.ring_buffer_size` | 1,048,576 | MPMC ring buffer capacity |
| `engine.dispatcher_workers` | 256 | Parallel message-processing goroutines |
| `wal.sync_interval_ms` | 10 | Max delay between fdatasync calls |

## Load Testing

```bash
./bin/loadtest -addr ws://host:8080/ws \
  -connections 1000000 \
  -ramp-rate 10000 \
  -msg-rate 1 \
  -duration 120s
```

Run across multiple machines to exceed single-host ephemeral-port limits.
