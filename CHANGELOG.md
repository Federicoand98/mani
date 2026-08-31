# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the version is `0.x`, breaking changes may land in any minor release.

## [Unreleased]

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

[Unreleased]: https://github.com/Federicoand98/mani/compare/v0.1.3...HEAD
[0.1.3]: https://github.com/Federicoand98/mani/releases/tag/v0.1.3
[0.1.2]: https://github.com/Federicoand98/mani/releases/tag/v0.1.2
[0.1.1]: https://github.com/Federicoand98/mani/releases/tag/v0.1.1
