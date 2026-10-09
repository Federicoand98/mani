---
title: HTTP and WebSocket API
description: The agent server's routes, request and response bodies, WebSocket message types and the permission flow.
weight: 5
---

```bash
export MANI_SERVER_TOKEN=secret
mani serve --config agent.yaml --addr :9000
```

Two planes. REST is the control plane: create a session, run a turn, read the journal. The
WebSocket is the data plane: streaming tokens, tool calls, and the approvals that only work when
somebody is connected.

Every route sits behind bearer auth. All examples assume:

```bash
TOKEN="Authorization: Bearer $MANI_SERVER_TOKEN"
```

| Method | Route | Purpose |
|---|---|---|
| `POST` | `/sessions` | create a session, with its own runtime |
| `GET` | `/sessions` | list active session ids |
| `DELETE` | `/sessions/{id}` | close a session and release its resources |
| `POST` | `/sessions/{id}/chat` | one turn on a session, no streaming |
| `POST` | `/chat` | one stateless turn |
| `GET` | `/sessions/{id}/turn` | WebSocket: streaming turns and approvals |
| `GET` | `/runs` | list runs |
| `GET` | `/runs/{id}` | one run with its events |

## `POST /sessions`

No body. Builds a runtime from the server's manifest.

```bash
curl -s -XPOST -H "$TOKEN" localhost:9000/sessions
```

```json
{ "session_id": "a1b2c3d4e5f6a7b8" }
```

`201 Created`.

## `GET /sessions`

```json
{ "sessions": ["a1b2c3d4e5f6a7b8"] }
```

## `DELETE /sessions/{id}`

`204 No Content`, or `404 Not Found` for an unknown id. Closing a session also closes its MCP
connections.

A session idle for 30 minutes is closed when the next one is created. A connected WebSocket
counts as in use for as long as it is connected, so an idle client does not lose its session.

## `POST /chat` and `POST /sessions/{id}/chat`

One turn over plain HTTP, no streaming. `/chat` is stateless: it builds a throwaway runtime for
the request.

```json
{ "input": "summarize README.md" }
```

```json
{ "output": { "response": "..." }, "usage": { "input": 1120, "output": 88 } }
```

`output` carries the structured result when the manifest declares an `output.schema`; otherwise
the text arrives as `{"response": "..."}`. A failure answers with:

```json
{ "error": "...", "is_error": true }
```

Permission requests are denied automatically on these routes: there is no channel to ask on.
Write the manifest with `allow` and `deny` if this is the route you use.

## `GET /runs` and `GET /runs/{id}`

```bash
curl -s -H "$TOKEN" "localhost:9000/runs?limit=5"
curl -s -H "$TOKEN" "localhost:9000/runs?status=error&since=24h"
curl -s -H "$TOKEN" "localhost:9000/runs/82fdb6ffaa1b"
```

Filters: `session`, `limit`, `status` (`ok`, `error`, `cancelled`) and `since` (a duration such as
`24h`). The list returns run headers with their counters; `/runs/{id}` returns the full record
including every event.

Both answer `501 Not Implemented` unless `observability.journal.path` is set. The view spans
sessions, so it is served from the shared journal and there has to be one. The same filters work
offline with `mani runs --status error --since 24h`.

## The WebSocket

```
GET /sessions/{id}/turn
```

One socket carries many turns: send an input, read the stream, and the turn ends with `done`,
`error` or `cancelled`. The socket then goes back to idle and accepts the next input. Messages
are JSON, one per frame.

### Client to server

| `type` | Fields | When |
|---|---|---|
| `input` | `input` | starts a turn; rejected while one is running |
| `permission_response` | `request_id`, `decision` | answers a `permission_request` |
| `cancel` | — | interrupts the running turn |

`decision` is `allow_once`, `allow_always` or `deny`. Any other value counts as `deny`.

```json
{ "type": "input", "input": "which tools do you have?" }
{ "type": "permission_response", "request_id": "9f8e7d", "decision": "allow_once" }
{ "type": "cancel" }
```

### Server to client

| `type` | Payload | Meaning |
|---|---|---|
| `token` | `{ text }` | a piece of the answer, streaming |
| `thinking` | `{ text }` | reasoning, when the model and config allow it |
| `tool_call` | `{ name, input }` | a tool is about to run |
| `tool_result` | `{ name, result, is_error }` | how it went |
| `usage` | `{ input, output }` | tokens consumed |
| `permission_request` | `{ request_id, tool_name, risk_level, input, preview }` | approval needed |
| `structured` | the schema object | the structured result, when a schema is declared |
| `done` | — | the turn finished |
| `error` | `{ message }` | the turn failed |
| `cancelled` | — | the turn was interrupted |

Payloads sit under `payload`:

```json
{ "type": "token", "payload": { "text": "I have " } }
{ "type": "tool_result", "payload": { "name": "read", "result": "...", "is_error": false } }
{ "type": "done" }
```

## The permission flow

A tool whose policy is `ask` suspends the turn until the client answers. The internal answer
channel never crosses the wire: the server mints a `request_id`, keeps the mapping, and your
reply with the same id unblocks the tool.

```json
{ "type": "permission_request", "payload": {
    "request_id": "9f8e7d", "tool_name": "bash", "risk_level": "execute",
    "input": { "cmd": "ls -la" }, "preview": "ls -la" } }
```

Three safety nets, all fail-closed:

- A disconnection denies everything still pending on that connection.
- A client that stays connected but never answers is denied after **10 minutes**.
- An unknown or repeated `request_id` is ignored, so a late answer cannot decide the next request.

A denied call is not fatal: the model is told the call was refused and carries on, and the
journal records it with the rule that stopped it.

## A full session

```bash
TOKEN="Authorization: Bearer $MANI_SERVER_TOKEN"

SID=$(curl -s -XPOST -H "$TOKEN" localhost:9000/sessions | jq -r .session_id)

websocat -H="$TOKEN" "ws://localhost:9000/sessions/$SID/turn" <<'EOF'
{"type":"input","input":"list the Go files and summarize the packages"}
EOF

curl -s -XDELETE -H "$TOKEN" localhost:9000/sessions/$SID
```

Next: [events and hooks](../events/).
