# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the version is `0.x`, breaking changes may land in any minor release.

## [Unreleased]

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

[Unreleased]: https://github.com/Federicoand98/mani/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/Federicoand98/mani/releases/tag/v0.1.2
[0.1.1]: https://github.com/Federicoand98/mani/releases/tag/v0.1.1
