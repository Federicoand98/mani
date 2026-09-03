# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the version is `0.x`, breaking changes may land in any minor release.

## [Unreleased]

## [0.1.4] - 2026-09-03

Bug-fix release. Three features of 0.1.3 turned out not to do what they said:
`${VAR}` expanded nothing, a manifest with two webhooks ran only one, and a
daily trigger drifted by an hour twice a year. The agent server also stops
leaking sessions and stops hanging on an unanswered permission.

### Added

- **Multiple webhook triggers.** A manifest can now declare several `webhook`
  triggers: they share one listener and get one route each, via the new
  `run.triggers[].path` key (default `/hook`).
- **`run.triggers[].token`** — the bearer token is declared per route, as a
  `${VAR}` reference, so two webhooks can hold different secrets and revoking
  one leaves the others working. When the field is absent the trigger falls
  back to `MANI_WEBHOOK_TOKEN`, so manifests written before 0.1.4 keep working
  unchanged.
- `mani validate` rejects two webhook triggers sharing a `path`, webhook
  triggers declaring different `addr` values (the listener is one), and a
  `path` that does not start with `/`.
- **Session garbage collection in the agent server.** A session idle for more
  than 30 minutes is closed and removed when the next one is created. An open
  WebSocket counts as in use for as long as it is connected, so a connected
  client is never collected mid-turn.

### Changed

- **`app.Daemon.Webhook` takes a webhook spec instead of four strings.**
  Breaking for library users; the CLI is unaffected.
- **An unanswered `permission_request` now resolves to deny after 10 minutes**
  instead of suspending the turn forever. Disconnecting still denies every
  pending request immediately.
- **`mani runs <id>` accepts a run id prefix.** The listing prints ids
  truncated to 12 characters, and passing one back used to fail because the
  lookup required the full id. An ambiguous prefix is reported as such.

### Fixed

- **Only the last webhook trigger existed.** `Daemon` held a single address,
  prompt, memory and token, and `BuildDaemon` overwrote them once per trigger,
  so a manifest with two webhooks silently ran only the second.
- **The journal could not tell webhooks apart.** Every webhook task was
  recorded with the literal trigger name `webhook`, discarding the id computed
  from the trigger. Tasks now carry their own trigger id.
- **`${VAR}` in a manifest was never expanded.** `expandEnvNodes` did not
  handle the document node returned when unmarshalling into a `yaml.Node`, so
  the walk stopped before reaching any value: references reached the runtime
  verbatim and an undefined variable was not reported. The feature shipped
  inert in 0.1.3.
- **Daily triggers drifted by an hour across a daylight-saving boundary.**
  `nextOccurrence` added a fixed 24 hours, but a day lasts 23 or 25 hours
  around a transition; the same bug, mirrored, affected `catch_up`. Both now
  use calendar arithmetic.
- **`at` silently accepted trailing junk.** `fmt.Sscanf` ignored whatever
  followed the pattern, so `at: "09:00 UTC"` scheduled 09:00 local time. It is
  now a validation error, and an invalid time refuses to start the daemon
  instead of dropping the trigger with a warning.
- **`Summary.Blocked` never counted permission denials.** The manifest policy
  hook recorded the action as `denied` while the counter matched `deny`, so a
  run blocked by `policy.tools` was reported as clean.

## [0.1.3] - 2026-08-31

Consolidation release: one security fix, one observability fix, two usability
gaps, and a CLI view over the run journal. No manifest key changed.

### Added

- **`mani runs`** — the run journal from the terminal, not only over HTTP.
  `mani runs` lists past runs (id, status, duration, tokens, tools, blocked);
  `mani runs <id>` replays one as a readable timeline, with subagent events
  indented. Accepts a unique id prefix like `git`. Filters with `--status` and
  `--since`, and `--json` for pipes. Reads the journal path from `--config`, or
  from `--path` directly.
- **`${VAR}` in the manifest** — secrets are referenced, not written:
  `env: { API_TOKEN: ${DEPLOY_TOKEN} }`. Manifests are meant to be committed.
  Only braced `${VAR}`, only scalar values, never keys, and never inside block
  scalars (`prompt: |` is prose). An undefined variable is an **error**, not an
  empty string — a blank token would silently mean "authentication disabled".
  `mani validate` checks it, so CI fails instead of production.
- `GET /runs` accepts `?status=` and `?since=`, matching the new CLI filters.
- `mani run --insecure`, to start webhook triggers without authentication.

### Changed

- **Webhook triggers now require authentication.** A `webhook` trigger needs
  `MANI_WEBHOOK_TOKEN`, and the daemon **refuses to start** without it.
  Previously `POST /hook` accepted any request: anyone able to reach the port
  could enqueue a run, with the request body flowing into the prompt — prompt
  injection with tool access. Pass `--insecure` to `mani run` for the old
  behaviour. `mani serve` has behaved this way since 0.1.0; this removes the
  inconsistency, which was worse than the hole itself — one strict HTTP surface
  and one open one invites trusting both.
- The webhook request body is capped at 64 KB. It was unbounded.

### Fixed

- **Blocked tool calls are recorded again.** Tracing and the journal were
  registered *after* the policy hooks, and `PreToolUse` hooks form a chain that
  stops at the first error — so a denied call reached neither the logs nor the
  journal. The single event an operator most wants to see was the one that
  vanished. Hook registration is now ordered observation → mutation → decision.
  In `PostToolUse` the order is deliberately reversed: redaction mutates the
  result and must run *before* observation, or the journal keeps secrets in clear.
- **The `blocked` and `masked` counters were always zero.** The journal matched
  `action` against `denied`/`masked` while the writer emits `deny`/`mask`.
- **A corrupted journal file appeared as a phantom run** with no date and no
  status: a file with zero readable events is now skipped instead of listed.
- `mani --help` printed the first command indented and the rest flush left: the
  header ended with a tab written past the `tabwriter`. Also two typos in the
  command summaries.

### Security

- The bearer-token check is now a single implementation (`app.BearerAuth`) shared
  by the agent server and the webhook trigger, instead of living only in
  `server/`. Two copies of a security check drift.

## [0.1.2] - 2026-08-14

First public release with downloadable binaries. Functionally identical to
`0.1.1` — what changed is the release pipeline, which was broken for the two
preceding tags. **Start here.**

### Added

- **Declarative manifest** with eight blocks, each answering one question:
  `identity`, `capabilities`, `context`, `output`, `policy`, `limits`, `run`,
  `observability`. Unknown keys are rejected instead of ignored.
- **Providers**: Ollama, OpenAI, Anthropic, GitHub Copilot, OpenRouter, and any
  OpenAI-compatible endpoint. A manifest naming a provider that cannot be reached
  fails to start rather than falling back to another model.
- **Built-in tools**: `read`, `write`, `edit`, `delete`, `glob`, `grep`, `bash`,
  `fetch`, `planning`, `delegate`.
- **Custom tools** as subprocesses (JSON on stdin, result on stdout) and through MCP.
- **Governance**: per-tool `allow` / `ask` / `deny`, risk levels including `network`,
  a domain allowlist for network tools with SSRF protection, rules and redaction,
  and per-run ceilings on tokens, tool calls, duration and iterations.
- **Structured output**: declare `output.schema` and the agent returns validated JSON.
- **Unattended execution**: cron and daily triggers driven by an in-process scheduler
  that runs on Linux, macOS and Windows; a durable task queue that survives restarts,
  with retries, backoff and a dead-letter area.
- **Run journal**: every run leaves a readable record on disk, queryable over HTTP.
- **Agent server** over HTTP and WebSocket with bearer authentication, multi-turn
  conversations and a permission back-channel.
- **Interactive terminal chat** with sessions, streaming, and image attachments.
- **CLI**: `run`, `serve`, `init`, `validate`, `tui`, plus `--help` and `--version`,
  with distinct exit codes for usage errors and runtime failures.
- **`!include`** for long system prompts: `prompt: !include ./prompts/reviewer.md`,
  resolved relative to the manifest and checked by `mani validate`.
- **Library use**: import `github.com/Federicoand98/mani` and wire the core with
  your own adapters.

### Fixed

- The release workflow refused to run: it fetched with `--depth=0`, which git
  rejects. It now also verifies that a tag sits on the **tip** of `master` —
  the previous check only asked whether the commit was somewhere in master's
  history, which is why `0.1.0` could be tagged on the initial commit.

## [0.1.1] - 2026-08-14

Retracts `0.1.0`. No functional change. Installable, but published without
binaries because the release workflow was still broken.

## [0.1.0] - 2026-08-14 — RETRACTED

Tagged on the repository's initial commit by mistake: the module contains no
package, so `go install github.com/Federicoand98/mani/cmd/mani@v0.1.0` fails.
The version is marked `retract` in `go.mod` and is skipped by `@latest`.
A published version cannot be withdrawn from the module proxy, only marked.

Use `0.1.2` or later.

[Unreleased]: https://github.com/Federicoand98/mani/compare/v0.1.4...HEAD
[0.1.4]: https://github.com/Federicoand98/mani/releases/tag/v0.1.4
[0.1.3]: https://github.com/Federicoand98/mani/releases/tag/v0.1.3
[0.1.2]: https://github.com/Federicoand98/mani/releases/tag/v0.1.2
[0.1.1]: https://github.com/Federicoand98/mani/releases/tag/v0.1.1
