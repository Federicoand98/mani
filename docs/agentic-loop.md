# Agentic loop & hook lifecycle

A map of `mani`'s agent loop, with the exact points where **hooks** fire, where the
**permission gate** acts, and where **events** are emitted towards the UI.

## Legend

| Symbol | Meaning |
|---|---|
| **Hook** (yellow) | a middleware point. It can observe, **mutate** the payload, or abort (`error`) |
| **Gate** (red) | NOT a hook: a mechanism of its own, deciding on the *final* input (post-mutation) |
| **Event** (blue) | emitted by the `Emitter` towards the UI. It does not interrupt the loop |
| grey | an internal step of the Agent |

Two layers: the **loop** hooks (`PreLLMCall`, `ContextFull`, `PostLLMCall`, `PreToolUse`,
`PostToolUse`) are fired by the **core** inside `Run`. The **session** hooks (`SessionStart`,
`SessionEnd`) are fired by the orchestrator (**app/Runtime**) at the session boundaries.

---

## 1. Session lifecycle (app)

```mermaid
flowchart LR
    classDef hook fill:#ffe8b3,stroke:#d99100,color:#5c3d00,stroke-width:2px;
    classDef step fill:#eeeeee,stroke:#888888,color:#222222;

    A["startup · NewSession · SwitchSession"]:::step
    SS["HOOK SessionStart"]:::hook
    T1["Execute (turn 1)"]:::step
    T2["Execute (turn 2)"]:::step
    DOTS["…"]:::step
    Q["switch away · /quit"]:::step
    SE["HOOK SessionEnd"]:::hook

    A --> SS --> T1 --> T2 --> DOTS --> Q --> SE
```

Each `Execute` expands into the loop of diagram 2.

---

## 2. The agent loop (core) — inside one `Execute`

```mermaid
flowchart TD
    classDef hook fill:#ffe8b3,stroke:#d99100,color:#5c3d00,stroke-width:2px;
    classDef event fill:#d6eaff,stroke:#3b82c4,color:#0b3a5c;
    classDef gate fill:#ffd6d6,stroke:#c43b3b,color:#5c0b0b;
    classDef step fill:#eeeeee,stroke:#888888,color:#222222;

    IN(["User input"]):::step
    ADD["memory.Add(user)"]:::step
    LOOP{{"Loop · max iterations"}}
    PRE["HOOK PreLLMCall<br/>mutates the outgoing messages"]:::hook
    EST["EstimateTokens(messages)"]:::step
    CHK{"estimate &gt; 80% of the limit?"}
    CF["HOOK ContextFull<br/>compaction · mutates the messages"]:::hook
    SEND["Client.Send → streaming"]:::step
    SEV["EventThinking · EventToken"]:::event
    POST["HOOK PostLLMCall<br/>observes/mutates the response"]:::hook
    ADDA["memory.Add(assistant)"]:::step
    SR{"StopReason?"}
    DONE["EventDone"]:::event
    EERR["EventError"]:::event
    RET(["end of turn"]):::step

    TPRE["HOOK PreToolUse<br/>mutates Input · may abort"]:::hook
    GATE["GATE Permission gate<br/>decides on the final Input"]:::gate
    PREQ["EventPermissionRequest"]:::event
    TBLK["memory.Add(blocked result)"]:::step
    TCALL["EventToolCall"]:::event
    TEXEC["executor.Execute(Input)"]:::step
    TPOST["HOOK PostToolUse<br/>observes/mutates result"]:::hook
    TRES["EventToolResult"]:::event
    TADD["memory.Add(tool result)"]:::step

    IN --> ADD --> LOOP --> PRE --> EST --> CHK
    CHK -- "yes" --> CF --> SEND
    CHK -- "no" --> SEND
    SEND -. "stream" .-> SEV
    SEND --> POST --> ADDA --> SR
    SR -- "end_turn" --> DONE --> RET
    SR -- "max_tokens" --> EERR --> RET
    SR -- "tool_use" --> TPRE --> GATE
    GATE -. "if risky" .-> PREQ
    GATE -- "denied" --> TBLK --> LOOP
    GATE -- "allowed" --> TCALL --> TEXEC --> TPOST --> TRES --> TADD --> LOOP
```

**Orderings that matter:**
- *Injection before estimation*: `PreLLMCall` prepends the system prompt, then `EstimateTokens`
  counts — so the system prompt is part of the count.
- *Compaction after estimation, before Send*: `ContextFull` trims the actual context you are
  about to send.
- *Mutate, then gatekeep*: `PreToolUse` may rewrite `Input`, and **then** the permission gate
  decides on that final input → no TOCTOU.

---

## 3. Hook reference table

| Hook | When it fires | Layer | Can mutate | On `error` | Payload |
|---|---|---|---|---|---|
| **SessionStart** | session created/switched | app | — | best-effort (ignored) | `*SessionEventPayload` |
| **PreLLMCall** | before every `Send` | core | `Messages` | aborts the Run | `*core.PreLLMCallPayload` |
| **ContextFull** | estimate > 80% of the limit (after injection) | core | `Messages` | aborts the Run | `*core.ContextFullPayload` |
| **PostLLMCall** | after the response, before storing it | core | `Response` | aborts the Run | `*core.PostLLMCallPayload` |
| **PreToolUse** | before every tool (phase 1) | core | `Input` | blocks *that* tool (`continue`) | `*core.PreToolUsePayload` |
| **PostToolUse** | after the tool executed | core | `Result`, `IsError` | aborts the Run | `*core.PostToolUsePayload` |
| **SessionEnd** | switch away / quit | app | — | best-effort (ignored) | `*SessionEventPayload` |

> The **permission gate** is not in the table because it is not a hook: it is invoked by the
> Agent *after* `PreToolUse`, on the final input. An observational hook (logging) must always
> return `nil` — an `error` means "stop".

---

## 4. Emitted events (towards the UI)

Distinct from hooks: **events** flow from the `Emitter` → channel → TUI for rendering, and do
not alter the loop.

`EventThinking` · `EventToken` · `EventToolCall` · `EventToolResult` ·
`EventPermissionRequest` · `EventUsage` · `EventDone` · `EventError` · `EventCancelled`
