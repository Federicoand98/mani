# mani

A micro-agent framework in Go for building LLM agents. Hexagonal architecture:
the domain (`core`) knows nothing about providers, transport, the filesystem or the UI.

## Language

**Agent**:
The domain loop that, given a memory and an input, calls the LLM and executes tools until
the model ends the turn. Lives in `core`, knows no concrete adapter.

**Runtime**:
The composition root of the CLI: wires Agent, provider, tools and permissions, and exposes
execution as a stream of `Event`. It is an application adapter, not domain. A library user
skips it and wires `core` directly.
_Avoid_: Engine, Orchestrator, App.

**Manifest**:
The YAML file that *declares* an agent. It is the product: the thesis is "agents as
configuration". It has **8 top-level blocks**, each answering **exactly one question**
(see § Manifest grammar). It is loaded into a `RuntimeSpec` with `KnownFields(true)`: an
unknown key is an **error**, never a silent no-op. _Avoid_: Config (that's the global config
in `config/`, a different thing), Spec (that's the Go type, not the document), Definition.

**Block**:
One of the 8 top-level sections of the Manifest. One block = one question; if a key answers
two, it must be split; if it answers none, it is not a manifest key.
_Avoid_: Section, Group, Namespace.

**Policy**:
The block answering "what is it allowed to do", at **three levels of granularity**: `tools`
(whether a tool may be used at all: allow/ask/deny), `rules` (whether **this invocation**
passes, by inspecting the input), `redact` (masking of the **output**). The mechanisms differ
— the first is a gate, the other two are hooks — but they answer a single question, so they
live in a single block.
_Avoid_: Permissions (that's only the first level), Guardrails (the former name of the others).

**Limits**:
The block of **numeric ceilings** on a run: tokens, tool calls, duration, iterations, per-tool
timeout, subagent depth. They used to be scattered across four places.
_Avoid_: Budget (the former, partial name), Quota, Constraints.

**Capability**:
An ability of the agent declared under `capabilities`: a Tool (built-in, subprocess or MCP),
a Subagent, the workspace. `planning` and `delegate` **are tools**, not flags: they are
declared like any other. A tool's manifest key and its runtime `tool.Name()` must be
identical — if they diverge, `Policy` stops blocking and Subagents fail to resolve, both
silently.
_Avoid_: Feature (the former catch-all with no criterion), Plugin, Skill.

**Trace**:
The structured sequence of steps in a run (llm call, tool call/result, correlated by `run_id`),
emitted as levelled `slog` output (error/warn/info/debug). Not in `core`: it is a cross-cutting
observer built **on the existing hooks** (`RegisterTracing` in `app`). The `run_id` travels in
the `context`. _Avoid_: Log (that's the medium), Span (this is not OpenTelemetry).

**MCP**:
Model Context Protocol: a standard for exposing tools as external processes/servers. mani is an
**MCP client** (`tool/mcp`): it connects to a server (stdio or HTTP/SSE) through the official
SDK, lists its tools and adapts them to `tool.Tool`. It makes tools writable in any language.
It is a tool-source adapter, like `tool/fs`. _Avoid_: Plugin, Extension.

**Trigger**:
A source of events (`every` at an interval, `daily` at a wall-clock time, `webhook` over HTTP)
that, when it fires, enqueues a `Task`. It is a **driving adapter** (the `Daemon` in `app`)
that drives the `Runtime` from events instead of from a human. It has a stable identity
(`name`, or a hash of type+schedule+prompt) that enables **catch-up** of firings missed while
the process was down. With no human present, permission prompts follow a **Policy** (deny by
default, allow opt-in). _Avoid_: Hook (that's loop middleware), Cron (only one of the types).

**Task**:
A single execution of the agent produced by a Trigger firing: it carries the prompt, the
originating trigger, the memory mode and the retry state (`attempt`, `next_at`). It is
**durable**: it survives a restart, so everything needed to re-run it lives in the Task itself.
There is (for now) no external way to enqueue one.
_Avoid_: Job, Run (the Run is the execution, the Task is the request).

**Task Queue**:
The port that enqueues, delivers and archives `Task`s. Two adapters: in-memory (the default)
and on the filesystem in **maildir** style — a task's state *is the directory it sits in*
(`pending/`, `running/`, `done/`, `failed/`) and a transition is an atomic `os.Rename`. You
inspect it with `ls` and repair it with `mv`. The opposite choice from the `Journal`, for a
precise reason: the journal is a historical record (append-only), the queue is **mutable state**.
_Avoid_: Broker, Bus, Mailbox.

**Scheduler**:
The `run.scheduler` manifest block and the workers that follow from it: it governs **how** Tasks
are executed (how many in parallel, how many may pile up, how many times to retry, whether they
survive a restart). It **contains no tasks**: those are born at runtime from Triggers. Its
workers start on their own — nothing is declared to consume the queue.
_Avoid_: Queue (as a block name: it describes the container, not the behaviour), Pool.

**Emitter**:
The port (in `core`) through which the Agent communicates outward what it produces while it
runs: tokens, reasoning, tool calls and their results. It speaks only strings and
`map[string]any` — it knows nothing of channels, events or UI. The channel adapter lives in
`app`. _Avoid_: Handler, Listener, Sink, Callback.

**Tool**:
A capability the Agent can invoke (read a file, run bash). It declares a name, a schema and a
`Risk Level`. Defined in package `tool`, consumed by the adapters.

**Risk Level**:
The danger a Tool declares about itself: `none`, `write`, `execute`. Determines whether a
permission is required before execution. Lives in `core`.
_Avoid_: Danger, Severity, Permission level.

**Hook**:
Middleware registered on the Agent, invoked at precise points of the lifecycle (pre/post tool,
pre/post LLM call in the core; session start/end on the orchestrator side). It receives a
`HookEvent` (`Type` + pointer payload) and may observe, **mutate** the data in place, or abort
by returning an `error`. Uniform: every hook receives every event and filters on `Type`. The
payload is valid only for the duration of the call.

**Registration order is semantic, not cosmetic.** `PreToolUse` hooks form a chain that stops at
the first one returning an error, so they are registered **observation → mutation → decision**:
tracing and the journal first (they always return `nil` and must see everything), then the hooks
that rewrite the payload, then policy, network and budget, which can abort. Register them the
other way round and a denied tool call reaches neither the logs nor the journal — the one event
an operator most wants to see is the one that disappears.

In `PostToolUse` the order is **reversed**: redaction *mutates* the result, so it must run before
observation, or the journal keeps the secrets in clear. Two chains, one principle applied in
opposite directions.
_Avoid_: Filter, Interceptor, Listener.

**HookEvent**:
What a Hook receives: a `Type` (an open string — the core declares the loop events, the
orchestrator may add its own, e.g. session) and a typed pointer `Payload`, mutable in place.
_Avoid_: Signal, Message.

**Compaction**:
The reduction of message history when the token estimate crosses a fraction of the context
window. It is not built into the Agent: it is a *strategy* implemented by a `ContextFull` hook
(which mutates the messages in place). The Agent merely estimates the tokens and fires the event.
_Avoid_: Truncation, Summarization (those are specific compaction strategies).

**Permission Manager**:
The gate that, before executing a tool, turns a `Risk Level` into a request to the user and
waits for the `Decision`. **It is not a generic Hook**: it is a mechanism of its own, invoked by
the Agent *after* the `PreToolUse` hooks (which may have mutated the input), so it decides on the
final input (no TOCTOU). It keeps the session state of what is "always allowed". Lives in `app`,
it is not domain.

**Decision**:
The user's answer to a permission request: `Deny`, `AllowOnce`, `AllowAlways`. An application
concept (`app`), never exposed to `core`.
_Avoid_: Permission, Choice, Answer.

**Diff Preview**:
The `+`/`-` rendering of a change a write tool is about to apply, derived from the `input`
(e.g. `old_content`/`new_content` of `edit`) **before** executing the tool. It travels in the
`Preview` field of the permission request and the TUI colours it, so the user approves or
rejects while seeing the change. Not a mechanism of its own: an enrichment of the permission
gate. _Avoid_: Patch, Hunk.

**Tool Output Truncation**:
The clipping of a tool's output beyond a byte limit (head+tail with a marker) before it reaches
`Memory`, to avoid saturating the context. Not in `core`: it is a default `PostToolUse` hook
registered by `app` (mutating `Result` in place). _Avoid_: Trim, Clip (Trim is already the
Compaction strategy).

**Event**:
A unit of the asynchronous `Runtime → UI` stream: token, reasoning, tool call/result, permission
request, done, error. An application concept. The UI consumes it to render. Distinct from the
`Emitter`, which is the port on the domain side.

**Memory**:
The sequence of messages of the current turn passed to the LLM. A port in `core`; the default
implementation is in-memory.
_Avoid_: History, Context, Conversation.

**Session**:
A distinct conversation, with its `Memory`, a `Plan` (todo) and metadata (id, title, timestamps,
model), switchable during execution and restorable from disk. An orchestration concept: it lives
in package `session/`, `core` does not know it.
_Avoid_: Conversation, Chat, Thread.

**Subagent**:
A child `core.Agent` spawned by the `delegate` tool for a sub-task: fresh memory, the parent's
tools (including `delegate`), inherited permission gate, silent output (nil Emitter). It returns
only its final answer to the parent (a `tool_result`) → it isolates the context. The nesting
depth travels in the `context` and is bounded by a depth cap. Not a new type: composition of the
existing `core.Agent`, orchestrated in `app`.
_Avoid_: Worker, Child agent ("child" is fine in code), Actor.

**Plan**:
The todo list of the current task: a sequence of `PlanStep` (`description` + `status`:
pending/in_progress/done). It is **model-owned** — the model writes and updates it through the
`planning` tool — and **advisory**: the loop does not enforce it, it only guides. It lives in the
`Session` (so it persists) and is re-injected as a reminder on every LLM call. `core` does not
know it: it is orchestration (tool + hook in `app`).
_Avoid_: Tasks, Steps (those are the entries), Workflow.

**Session Store**:
The port that saves, loads, lists and deletes `Session`s. Lives in `session/`; it has an
in-memory adapter and a file one (one JSON per session). `core` does not know it: serialization
of `Message` lives entirely in the adapter (through DTOs).
_Avoid_: Repository, Persistence, DAO.

**Provider**:
A concrete LLM service (ollama, openai, anthropic, copilot, openrouter, or a custom
OpenAI-compatible endpoint). It is a *configuration choice*, not a type: the active `provider`
in the config selects which adapter to wire. Its config (`base_url` + `model`) lives in the
`providers` map, so **every Provider remembers its own model**; the active model is the one of
the active Provider. Several Providers may share the same `Wire Format`.
_Avoid_: Backend, Vendor, Engine.

**Wire Format**:
The concrete protocol an adapter uses to talk to the LLM (OpenAI Chat Completions vs Anthropic
Messages). It determines message/tool mapping and stream parsing. Copilot and Openrouter use
OpenAI's Wire Format with different endpoints and auth.
_Avoid_: Protocol, API style.

**Credential**:
The secret used to authenticate against a Provider: an API key (type `api`) or an OAuth token
with refresh and expiry (type `oauth`, e.g. Copilot). It lives **only** in `auth.json`
(`$XDG_DATA_HOME/mani/auth.json`, mode 0600), never in `config.json`. Managed in package
`config/`; `auth.json` is authoritative.
_Avoid_: Secret, Token, Key (those are specific cases).

**Command**:
A slash command of the TUI (`/model`, `/clear`, `/login`, …): it parses its arguments, acts on
the `Runtime` and returns a `Result` — synchronous output, or an `Action` asking the TUI to enter
a mode (picker, login). Lives in `tui/command`, consumed **only** by the TUI (the REPL was
deprecated). _Avoid_: Action (that's a field of Result), Handler, Verb.

**Model Lister**:
An *optional* capability of an adapter: listing the models available for the Provider. A
separate interface in `core` (`ModelLister`), not part of the `LLMClient` port. The `/model`
command type-asserts it: if the adapter implements it, a picker is shown, otherwise it degrades
to free text.
_Avoid_: ModelRegistry, Catalog.

---

## Manifest grammar

**The rule:** every top-level block answers **exactly one question**.

| Block | Question |
|---|---|
| `identity` | who THINKS? |
| `capabilities` | what can it DO? |
| `context` | what does it SEE and REMEMBER? |
| `output` | what does it RETURN? |
| `policy` | what is it ALLOWED to do? |
| `limits` | HOW MUCH may it consume? |
| `run` | WHEN does it start and HOW is it executed? |
| `observability` | what does it LEAVE BEHIND? |

**Where a new key goes.** Ask these questions *in this order*; the first "yes" wins. The order
runs from the least invasive (only observes) to the most fundamental (defines the agent), so the
decision is deterministic.

```
1. Does it only record, without changing behaviour?      → observability
2. Does it decide WHEN a run starts, or HOW MANY run?    → run
3. Is it a numeric ceiling on some consumption?          → limits
4. Can it BLOCK or MODIFY an action?                     → policy
5. Does it constrain the SHAPE of the final answer?      → output
6. Does it change what ends up in the model's CONTEXT?   → context
7. Does it add an ABILITY to the agent?                  → capabilities
8. Does it change WHO or WHAT reasons?                   → identity
```

The tree decides **where** something goes; the feature filter decides **whether** it should exist
at all (manifest expressiveness / safe autonomy / operability — otherwise it is LangChain turf).
Two distinct checks, both necessary.

**Retired names** (phase 31, clean break — `KnownFields(true)` rejects them explicitly):
`features` · `permissions` · `guardrails` · `budget` · `queue` · `mcpservers` · `system_prompt` ·
`output_schema` · `context_window` · `max_iterations` · `triggers` · top-level `tools`,
`provider` and `model`. The `todo_write` tool became `planning`; the `read_file`/`edit_file`/
`write_file` tools became `read`/`edit`/`write` so that a manifest key and a runtime tool name
are the same string.
