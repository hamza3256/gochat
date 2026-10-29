# GoChat

A single-process messaging engine. v0 is a load-testable WebSocket echo and small-room fan-out path. Clustering, persistence, and presence are not in this context yet.

## Language

**Connection**:
A live WebSocket session owned by exactly one reactor, identified by its OS file descriptor.
_Avoid_: socket, client, session (as the primary noun)

**Reactor**:
A pinned OS thread that owns a poller and every Connection accepted on its listener.
_Avoid_: event loop, worker, dispatcher

**Room**:
A set of Connections that receive each other's publications. A Connection is in at most one Room.
_Avoid_: channel, topic, group

**Echo**:
A frame the engine writes back on the same Connection, byte-for-byte in the WebSocket payload.
_Avoid_: ping (that's RFC 6455 ping/pong)

**Publication**:
A frame fanned out to every Connection currently in a Room, including the sender (so the sender can measure round-trip time).
_Avoid_: broadcast, message (until persistence exists)

**Envelope**:
The 5-byte application header inside a WebSocket binary payload: opcode, room id, then opaque bytes.
_Avoid_: message, packet, proto
