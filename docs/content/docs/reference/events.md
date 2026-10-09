---
title: Events and hooks
description: The events a run emits, the hook points in the loop, their payloads, and what an error from a hook does.
weight: 6
---

Two separate mechanisms, often confused. **Events** flow outward and never change the run.
**Hooks** sit inside the loop and can mutate or abort it.

## Events

A run emits events as it goes. The terminal chat, the agent server's WebSocket, `mani run` and
the journal are all consumers of the same stream.

| Event | Payload | Emitted |
|---|---|---|
| `EventThinking` | text | reasoning, when enabled |
| `EventToken` | text | each piece of the streamed answer |
| `EventToolCall` | name, input | before a tool runs |
| `EventToolResult` | name, result, is_error | after it returns |
| `EventPermissionRequest` | tool name, risk level, input, preview, reply channel | a gated call is waiting |
| `EventUsage` | input and output tokens | after a model call |
| `EventDone` | run id, result, text | the run finished |
| `EventError` | run id, error | the run failed |
| `EventCancelled` | — | the run was interrupted |

`EventPermissionRequest` carries the channel the runtime is waiting on. Whoever consumes the
stream decides: the chat asks you, the WebSocket forwards a `permission_request`, and every
headless consumer denies. That is why the permission policy is the same everywhere while the
answer comes from the transport.

From Go, the stream is a channel:

```go
for ev := range rt.Execute(ctx, "summarize the README") {
    switch ev.Type {
    case app.EventToken:
        fmt.Print(ev.Payload.(app.TokenPayload).Text)
    case app.EventDone:
        fmt.Println(ev.Payload.(app.DonePayload).Result)
    }
}
```

A new interface renders a run by consuming events. The loop does not know it exists.

## Hooks

| Hook | Fires | Layer | Can mutate | An error |
|---|---|---|---|---|
| `SessionStart` | a session is created or switched to | runtime | — | is ignored |
| `PreLLMCall` | before every model call | core | the messages | aborts the run |
| `ContextFull` | the token estimate passes 80% of the window | core | the messages | aborts the run |
| `PostLLMCall` | after the response, before it is stored | core | the response | aborts the run |
| `PreToolUse` | before every tool call | core | the tool input | blocks that call, the run continues |
| `PostToolUse` | after the tool returns | core | the result and its error flag | aborts the run |
| `SessionEnd` | switching away, or quitting | runtime | — | is ignored |

The payloads are mutable on purpose: `PreLLMCall` is how the system prompt and context injection
get in, `ContextFull` is how compaction trims, `PreToolUse` is how an argument is rewritten or
masked, `PostToolUse` is how a result is truncated.

{{< callout type="warning" >}}
  An error from a hook means "stop". A logging hook that returns a write failure will abort runs,
  so observational hooks return `nil`.
{{< /callout >}}

### The permission gate is not a hook

It runs after `PreToolUse`, on the input that hook may have rewritten. That order closes the
check-then-use gap: nothing can change an argument after it was approved.

### What governance is made of

`policy`, the pattern rules, redaction and the per-run budgets are hook registrations the runtime
makes while reading your manifest. The domain package has no knowledge of them, which is why
`core/` has zero external dependencies and why a transport cannot opt out of a policy.

## From Go

Hooks are public, so a program embedding mani can add its own:

```go
rt.OnPreToolUse(func(ctx context.Context, p *core.PreToolUsePayload) error {
    if p.ToolName == "bash" && strings.Contains(p.Input["command"].(string), "rm -rf") {
        return errors.New("policy: refused")       // blocks this call
    }
    p.Input["command"] = redact(p.Input["command"]) // or rewrite it
    return nil
})
```

`OnPostToolUse`, `OnPreLLMCall`, `OnPostLLMCall` and `OnContextFull` follow the same shape. A
custom policy is a hook: there is no separate policy API to learn.

Next: [the demos](../../demos/), or back to the [concepts](../../concepts/agent-loop/).
