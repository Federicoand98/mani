# Agentic loop & hook lifecycle

Mappa del ciclo dell'agente di `mani`, con i punti esatti in cui scattano gli **hook**,
dove agisce il **gate permesso** e dove vengono emessi gli **eventi** verso la UI.

## Legenda

| Simbolo | Significato |
|---|---|
| **Hook** (giallo) | punto di middleware. Può osservare, **mutare** il payload, o abortire (`error`) |
| **Gate** (rosso) | NON è un hook: meccanismo a sé, decide sull'input *finale* (post-mutazione) |
| **Evento** (azzurro) | emesso dall'`Emitter` verso la UI (REPL/TUI). Non interrompe il loop |
| grigio | Passo interno dell'Agent |

Due layer: gli hook di **loop** (`PreLLMCall`, `ContextFull`, `PostLLMCall`, `PreToolUse`,
`PostToolUse`) li spara il **core** dentro `Run`. Gli hook di **sessione** (`SessionStart`,
`SessionEnd`) li spara l'orchestratore (**app/Runtime**) ai confini della sessione.

---

## 1. Ciclo di vita della sessione (app)

```mermaid
flowchart LR
    classDef hook fill:#ffe8b3,stroke:#d99100,color:#5c3d00,stroke-width:2px;
    classDef step fill:#eeeeee,stroke:#888888,color:#222222;

    A["startup · NewSession · SwitchSession"]:::step
    SS["HOOK SessionStart"]:::hook
    T1["Execute (turno 1)"]:::step
    T2["Execute (turno 2)"]:::step
    DOTS["…"]:::step
    Q["switch away · /quit"]:::step
    SE["HOOK SessionEnd"]:::hook

    A --> SS --> T1 --> T2 --> DOTS --> Q --> SE
```

Ogni `Execute` espande il loop del diagramma 2.

---

## 2. Il loop dell'agente (core) — dentro un `Execute`

```mermaid
flowchart TD
    classDef hook fill:#ffe8b3,stroke:#d99100,color:#5c3d00,stroke-width:2px;
    classDef event fill:#d6eaff,stroke:#3b82c4,color:#0b3a5c;
    classDef gate fill:#ffd6d6,stroke:#c43b3b,color:#5c0b0b;
    classDef step fill:#eeeeee,stroke:#888888,color:#222222;

    IN(["User input"]):::step
    ADD["memory.Add(user)"]:::step
    LOOP{{"Loop · max 10 iterazioni"}}
    PRE["HOOK PreLLMCall<br/>muta i messaggi da inviare"]:::hook
    EST["EstimateTokens(messaggi)"]:::step
    CHK{"stima &gt; 80% del limite?"}
    CF["HOOK ContextFull<br/>compaction · muta i messaggi"]:::hook
    SEND["Client.Send → streaming"]:::step
    SEV["EventThinking · EventToken"]:::event
    POST["HOOK PostLLMCall<br/>osserva/muta la risposta"]:::hook
    ADDA["memory.Add(assistant)"]:::step
    SR{"StopReason?"}
    DONE["EventDone"]:::event
    EERR["EventError"]:::event
    RET(["fine turno"]):::step

    TPRE["HOOK PreToolUse<br/>muta Input · può abortire"]:::hook
    GATE["GATE Permission gate<br/>decide sull'Input finale"]:::gate
    PREQ["EventPermissionRequest"]:::event
    TBLK["memory.Add(blocked result)"]:::step
    TCALL["EventToolCall"]:::event
    TEXEC["executor.Execute(Input)"]:::step
    TPOST["HOOK PostToolUse<br/>osserva/muta result"]:::hook
    TRES["EventToolResult"]:::event
    TADD["memory.Add(tool result)"]:::step

    IN --> ADD --> LOOP --> PRE --> EST --> CHK
    CHK -- "sì" --> CF --> SEND
    CHK -- "no" --> SEND
    SEND -. "stream" .-> SEV
    SEND --> POST --> ADDA --> SR
    SR -- "end_turn" --> DONE --> RET
    SR -- "max_tokens" --> EERR --> RET
    SR -- "tool_use" --> TPRE --> GATE
    GATE -. "se a rischio" .-> PREQ
    GATE -- "negato" --> TBLK --> LOOP
    GATE -- "permesso" --> TCALL --> TEXEC --> TPOST --> TRES --> TADD --> LOOP
```

**Ordini che contano:**
- *Injection prima della stima*: `PreLLMCall` antepone il system prompt, poi `EstimateTokens` conta — così il system entra nel conteggio.
- *Compaction dopo la stima, prima del Send*: `ContextFull` taglia il contesto reale che stai per inviare.
- *Muta poi gatekeeper*: `PreToolUse` può riscrivere `Input`, **poi** il permesso decide su quell'input finale → niente TOCTOU.

---

## 3. Tabella di riferimento degli hook

| Hook | Quando scatta | Layer | Può mutare | Su `error` | Payload |
|---|---|---|---|---|---|
| **SessionStart** | creazione/switch sessione | app | — | best-effort (ignorato) | `*SessionEventPayload` |
| **PreLLMCall** | prima di ogni `Send` | core | `Messages` | abort del Run | `*core.PreLLMCallPayload` |
| **ContextFull** | stima > 80% del limite (dopo injection) | core | `Messages` | abort del Run | `*core.ContextFullPayload` |
| **PostLLMCall** | dopo la risposta, prima di salvarla | core | `Response` | abort del Run | `*core.PostLLMCallPayload` |
| **PreToolUse** | prima di ogni tool (fase 1) | core | `Input` | blocca *quel* tool (`continue`) | `*core.PreToolUsePayload` |
| **PostToolUse** | dopo l'esecuzione del tool | core | `Result`, `IsError` | abort del Run | `*core.PostToolUsePayload` |
| **SessionEnd** | switch away / quit | app | — | best-effort (ignorato) | `*SessionEventPayload` |

> Il **Permission gate** non è in tabella perché non è un hook: è invocato dall'Agent
> *dopo* `PreToolUse`, su input finale. Un hook osservativo (logging) deve sempre
> ritornare `nil` — `error` significa "ferma".

---

## 4. Eventi emessi (verso la UI)

Distinti dagli hook: gli **eventi** fluiscono dall'`Emitter` → canale → REPL/TUI per il
rendering, e non alterano il loop.

`EventThinking` · `EventToken` · `EventToolCall` · `EventToolResult` ·
`EventPermissionRequest` · `EventDone` · `EventError`
