---
title: Run it as a service
description: Expose a manifest over HTTP and WebSocket with bearer auth, streaming turns, and approvals answered over the wire.
weight: 7
---

Goal: other programs talk to your agent over HTTP, with streaming turns and the option to answer
permission prompts from a user interface.

Prerequisites: a manifest, and a token.

## Start it

```bash
export MANI_SERVER_TOKEN=$(openssl rand -hex 24)
mani serve --config agent.yaml --addr :9000
```

Without a token and without `--insecure`, the command refuses to start. Every route is behind
bearer auth, including the WebSocket.

```bash
curl -s -H "Authorization: Bearer $MANI_SERVER_TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"input":"summarize README.md"}' \
     localhost:9000/chat
```

```json
{ "output": { "response": "..." }, "usage": { "input": 1120, "output": 88 } }
```

`POST /chat` is stateless: it builds a throwaway runtime for the request. Use it from scripts and
cron jobs, where a session would be ceremony.

## Sessions, when the conversation matters

```bash
TOKEN="Authorization: Bearer $MANI_SERVER_TOKEN"
SID=$(curl -s -XPOST -H "$TOKEN" localhost:9000/sessions | jq -r .session_id)

curl -s -XPOST -H "$TOKEN" -H 'Content-Type: application/json' \
     -d '{"input":"what tools do you have?"}' \
     localhost:9000/sessions/$SID/chat

curl -s -XDELETE -H "$TOKEN" localhost:9000/sessions/$SID
```

A session owns its own runtime, so its memory, its MCP connections and its journal belong to it.
An idle session is closed after 30 minutes, when the next one is created; a connected WebSocket
counts as in use for as long as it is connected.

## Streaming, and approvals over the wire

```
WS /sessions/{id}/turn
```

One socket carries many turns. Send an `input` message, receive the event stream, and the turn
ends with `done`, `error` or `cancelled`. The socket then goes back to idle and accepts the next
input.

```json
{ "type": "input", "input": "tidy up the imports in main.go" }
```

What comes back, one JSON frame at a time: `token`, `thinking`, `tool_call`, `tool_result`,
`usage`, `structured` (when a schema is declared), then `done`.

This is also the one transport where `ask` works. A tool whose policy is `ask` suspends the turn
and the server sends:

```json
{ "type": "permission_request", "payload": {
    "request_id": "9f8e7d", "tool_name": "bash", "risk_level": "execute",
    "input": { "cmd": "ls -la" }, "preview": "ls -la" } }
```

Answer with the same id:

```json
{ "type": "permission_response", "request_id": "9f8e7d", "decision": "allow_once" }
```

`allow_once`, `allow_always` or `deny`; anything else counts as `deny`. Two safety nets sit
behind it: a disconnection denies everything pending, and a client that goes quiet is denied after
ten minutes. A run never waits forever on a human who left.

Full message tables are in the [HTTP and WebSocket reference](../../reference/http-api/).

## Reading the journal over HTTP

```bash
curl -s -H "$TOKEN" "localhost:9000/runs?limit=5"
curl -s -H "$TOKEN" "localhost:9000/runs?status=error&since=24h"
curl -s -H "$TOKEN" "localhost:9000/runs/82fdb6ffaa1b"
```

Same filters as `mani runs`, one vocabulary for HTTP and the terminal. Both routes answer
`501 Not Implemented` until `observability.journal.path` is set: the cross-session view is served
from the shared journal, so there has to be one.

## Putting it behind something

The server speaks plain HTTP and expects to sit behind whatever already terminates TLS for you.
Two things worth deciding before you expose it:

- **The token is the whole authentication story.** There are no users and no scopes, so treat the
  token as a service credential and rotate it like one.
- **`--insecure` is for development.** It starts the server with no auth and logs a warning that
  says so.

{{< callout type="warning" >}}
  An agent reachable over HTTP is an agent whose prompt comes from outside. Keep `policy` written
  as if every input were hostile: deny what the service does not need, and use pattern rules on
  what it does.
{{< /callout >}}

## What you should see

- `401` without a token, on every route including the WebSocket.
- A `done` frame at the end of each WebSocket turn, and the socket still usable afterwards.
- `mani runs --config agent.yaml` listing the turns with source `server`.
- A denied `ask` when nobody answers within ten minutes, recorded in the journal.

Next: the [HTTP and WebSocket reference](../../reference/http-api/).
