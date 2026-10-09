---
title: Troubleshooting
description: The errors mani produces, what each one means, and the fix.
weight: 6
---

Errors in the order you are likely to meet them.

## `provider openai: no creds, use /login openai before`

The manifest names a provider that has no credentials. Log in once, from the chat:

```bash
mani
/login openai
```

The key is written to `auth.json` (mode `0600`), never to the manifest and never to
`config.json`.

mani does not fall back to another model when a provider is unusable. Cost and privacy would then
differ from what you declared, and you would find out afterwards.

## `provider X: no base url in config`

The provider exists but has no `base_url`. The five built-in ones are configured by default; a
custom entry needs the URL:

```json
{ "providers": { "myhost": { "base_url": "http://10.0.0.5:8000/v1", "model": "qwen3" } } }
```

## `ollama: HTTP 404` or a connection refused

Ollama is not running, or the model is not pulled.

```bash
ollama serve &
ollama pull qwen3.5:9b
```

Any model you use needs tool-calling support. A model without it will answer in prose and never
call a tool, which looks like an agent that ignores its instructions.

## `field X not found in type app.SomeSpec`

An unknown key in the manifest, with its line number. Usually a near miss: `policy.tool` instead
of `policy.tools`, `limits.max_token` instead of `max_tokens`. Unknown keys are refused rather
than ignored, because the alternative is a rule that silently does nothing.

Check the [manifest reference](../reference/manifest/) for the exact spelling.

## `environment variable X is not set`

A `${VAR}` in the manifest has nothing behind it. An undefined variable is an error, never an
empty string: a blank bearer token would mean "authentication disabled" to the webhook listener.

```bash
DEPLOY_TOKEN=$(pass show mani/deploy) mani run --config watchdog.yaml
```

`mani validate` resolves them too, which is why it belongs in CI.

## `!include "./prompts/x.md": no such file or directory`

Include paths resolve against the **manifest**, not the working directory. Absolute paths are
refused, and files are capped at 256 KB.

## The agent never calls a tool

Check three things, in this order:

1. The tool is declared in `capabilities.tools`. Nothing is implicit, including `planning` and
   `delegate`.
2. `policy` does not deny it, and `policy.tools.default` is not `deny` with no exception for it.
3. The tool's `description` says when to use it. For a tool you declared yourself, that
   description is a prompt: a vague one is a tool the model never reaches for.

`mani runs <id>` shows whether the call happened and was blocked, or never happened at all.

## A tool call was blocked and nobody asked me

Headless runs — `mani run --task`, triggers, batch, flow, `mani mcp`, `POST /chat` — deny `ask`
automatically. There is no channel to ask on, and a run that waits forever for an absent human is
worse than one that stops with a reason.

Write unattended manifests with `allow` and `deny`. `ask` works in the terminal chat and over the
agent server's WebSocket, which are the two places a human is reachable.

## `agent: reached max iterations without completing the task (10)`

The loop hit `limits.max_iterations`. Either the task genuinely needs more steps, or the model is
circling: a schema it cannot satisfy, a tool that keeps failing, or a prompt that asks for
something impossible with the tools it has.

Read the run before raising the number:

```bash
mani runs --config agent.yaml <id>
```

A repeated failing tool call in the timeline means the limit is a symptom, not the problem.

## `no --task given and no triggers in the manifest`

`mani run` without `--task` starts the manifest's triggers. With no triggers declared there is
nothing to start, so the command refuses instead of exiting silently.

## `--task does not apply to a flow`

`mani run` dispatches on the file: a file whose top-level key is `flow:` is a flow. Flows take
their records from `--in` or from their first step; `--task`, `--image` and `--provenance` belong
to an agent. The reverse also holds: `--in`, `--out` and `--limit` on a manifest are usage errors.

## `x.json line 4: id "../x" cannot be a file name`

A batch or flow input record has an `id` that cannot become a file name, is missing, or is
repeated. The whole input is validated before the first model call, so a bad file costs nothing
instead of failing after six hundred successful runs.

## A batch reruns everything

Resuming works off `--out`: a record whose `<id>.json` is there is skipped. Without `--out` there
is nothing to skip, and a flow without `--out` keeps its records in a temporary directory that is
removed at the end.

## A flow redoes a step I did not change

Resuming follows file timestamps. Deleting a step's whole directory throws them away and redoes
everything downstream; deleting only its `.done` marker makes the step rerun while identical
records stay put:

```bash
rm out/fetch_letters/.done
```

Editing an input line in `--in` does not redo it either, because input records are identified by
`id`. Delete that record's result file to force it.

## `GET /runs` returns `501 Not Implemented`

The server has no journal path. The route spans sessions, so it is served from the shared
journal:

```yaml
observability:
  journal: { enabled: true, path: ./runs }
```

## `no token: set --token, MANI_SERVER_TOKEN, or pass --insecure`

`mani serve` will not start without authentication. `--insecure` exists for local development and
logs a warning saying so.

## An MCP client shows nothing, or disconnects

Three usual causes:

- The client needs an **absolute** `--config` path. It decides the working directory.
- `identity.name` is required under `mani mcp` and must match `^[a-zA-Z0-9_-]{1,64}$`.
- Something other than JSON-RPC reached stdout. mani keeps logs on stderr; a wrapper script that
  echoes a line will break the connection.

## `x509: certificate signed by unknown authority`

The server you are fetching serves an incomplete certificate chain: it sends its own certificate
but not the intermediate. Browsers and some HTTP clients paper over it by fetching the missing
certificate; Go does not.

The fix belongs on that server, which should serve the full chain. Until it does, point Go at a
bundle that contains the intermediate:

```bash
SSL_CERT_FILE=/path/to/fullchain.pem mani run --config agent.yaml --task "..."
```

Never disable verification to get past it. The credentials in the request travel over that
connection.

## Where to look next

```bash
mani validate --config agent.yaml     # the manifest, resolved, with no model call
mani run --config agent.yaml --task "..." --verbose   # logs on stderr
mani runs --config agent.yaml --status error --since 24h
mani runs --config agent.yaml <id>    # one run as a timeline
tail -f ~/.config/mani/mani.log       # the chat's log file
```

If the behaviour still looks wrong, it may be a bug: see [contributing](../contributing/).
