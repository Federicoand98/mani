# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the version is `0.x`, breaking changes may land in any minor release.

## [Unreleased]

Flows grow the three things real pipelines kept needing — parameters, filters and joins — and
`app/` is split along its boundaries, which is a breaking change for anyone importing mani as a
library.

### Added

- **Agent creation workbench.** A resizable configuration/trial split replaces the seven-step
  wizard. Instructions have Markdown source and preview; tool access uses Allow/Ask/Deny
  segments, with less-used fields under Advanced options. Provider and model both use dropdowns;
  model choices refresh for the selected provider without discarding existing unlisted values.
  Templates, imports, draft restoration
  and precise YAML edits remain intact. Trials still run real tools with inline approvals;
  changed settings require an explicit fresh trial instead of replaying actions.
  Workspace Browse uses an authenticated OS-native directory dialog and returns the absolute
  selected path, including outside the console folder. Manifest destinations remain confined
  to the console folder. Linux requires zenity or kdialog and a desktop session; unsupported
  environments report an error and keep manual path entry available.

- **Managed chat context.** Provider/model limits can be discovered or configured manually.
  Input and shared-context ceilings are distinct, with a configurable output reserve (4,096
  tokens by default). Console and terminal model-only chats summarize older complete turns at
  80% of the effective input budget, keeping recent turns intact and preserving stored history.
  Summary checkpoints survive restarts; summary calls count toward consumption and budgets.
  Manifests opt in with `context.compaction.mode: summary` and `keep_turns`; legacy `keep`
  retains its message-count meaning. Recognized overflows get one smaller request, never a
  restarted turn or repeated tool effects.

- **Flow parameters.** A flow declares `params:` and references them as `${params.name}` in any
  value; `mani run --set name=value` fills them, a `default` covers the rest.

  ```yaml
  params:
    busta: "The busta to rebuild"
    since: { about: "Earliest year to keep", default: "1490" }
  ```

  A required parameter with no value stops the run before anything starts, naming it and its
  description. `${params.x}` for an undeclared name is a load error with its line, and so is
  `--set x=…`: a typo is never ignored. `mani validate` lists the parameters, their values and
  where each came from, and still shows a flow whose required parameters are not set.

  **A state directory belongs to its parameters.** The first run with `--out` writes
  `<out>/.params.json`, and a later run with a different value — a different `--set`, or a
  default changed in the file — is refused. Without it, rerunning busta 13 in the directory of
  busta 12 found every step "already done" and handed back busta 12's records.

- **Filters on a step: `where:`.** `where: { confidence: low }` keeps only the records whose
  payload has those field values. Equality only — no comparisons, no `or`, no expressions: a
  threshold comes from the agent upstream declaring a band in its schema, so the judgement stays
  with the agent and the flow only routes.

  When the step it reads is an agent with an `output.schema`, the filter is checked at load time
  — a misspelt field, a value outside the enum or a type mismatch is an error with its line,
  instead of a flow that filters out everything and succeeds. A record left out is not pending:
  it is not run, not retried, and not a failure. A record that stops matching has its old result
  removed from the step's directory, and a whole-step run behind a filter reruns when any upstream
  record changes, kept or not.

- **Joins: `from_all: [a, b]`.** A step can read several steps; every record arrives with a
  `from` field naming the step it came from, so the same id in two sources stays two records. A
  join reruns when any of its sources changes. `for_each` over a list, and `where` on a join, are
  refused with a message saying what to write instead.

- `journal/journaltest` is public: the contract every `journal.Journal` adapter must pass, so an
  adapter written outside this repository can run the same checks. It now also covers
  `journal.Multi`, the fan-out the runtime actually uses, which no test covered before.

- **`mani web`, a console in the browser (first milestone).** One command opens the browser on
  the agents' folder: today it connects model providers and sets the theme; chat, agents, runs and
  flows follow in later releases. It is served by the mani binary itself, with nothing to install
  and no database: everything it shows lives in the files mani already uses.

  ```bash
  mani web                 # the current folder, http://127.0.0.1:7878, opens the browser
  mani web --dir ~/agents --no-open
  ```

  It listens on loopback only and refuses any other address. Each start prints a one-time link;
  opening it gives the browser a session cookie and the link stops working, so it is harmless in
  the shell history. Requests carrying a foreign `Host` (DNS rebinding) or another `Origin` are
  refused.

  The **Providers** page starts empty and adds Ollama, OpenAI, Anthropic, OpenRouter or GitHub
  Copilot: a provider is tested before it is saved, and a refusal is reported in the provider's
  words with nothing saved and no other provider tried in its place. Keys go to `auth.json`, as
  with `/login` in the TUI. A provider added from the browser is marked `"enabled": true` in
  `config.json`; existing files read as before, with every provider not enabled, so the page
  opens empty even for a TUI user with Ollama configured.

  `go install` builds mani without the console, because the frontend is compiled by `make web`
  and the release pipeline: `mani web` then shows a page saying so, and the API still answers.

- **Chat in `mani web` (second milestone).** Conversations with any connected model, streamed word
  by word: Markdown replies with a Copy button on code, reasoning in a closed block, images pasted
  or dropped into the message, and `/help`, `/clear`, `/model`, `/thinking`. Each conversation
  keeps its own provider and model, and can switch them halfway. The chat has no tools: the model
  answers alone, and tools arrive with agents.

  Conversations belong to the folder: they are saved in `<dir>/.mani/sessions`, next to the
  agents, and `.mani/` gets its own `.gitignore` so they stay out of a project's history. A reply
  does not depend on the browser tab: closing or reloading it mid-answer leaves the reply running,
  and it is saved when it ends. How many recent messages the model remembers is a setting (20 by
  default); older ones stay on screen, below a line that says so.

  The WebSocket messages of `mani serve` move to `server/wire`, and `mani web` speaks the same
  protocol. Nothing changes on the wire for existing clients.

- **Agents in `mani web` (third milestone, first half).** The Agents page lists every manifest in
  the folder and its subfolders, with its model, tools, triggers and state: ready, with problems,
  or not valid YAML. Agents can be duplicated, renamed (file and `identity.name` together) and
  deleted. The editor opens one on its eight blocks, each with a form, next to the YAML in an
  editable panel; fields and text follow each other both ways. Saving changes only the characters
  of the values you touched: comments, their alignment, key order, quoting and `!include` stay as
  written. Problems are listed with their line as you type, and each leads to its field. A file
  changed on disk reloads by itself when there are no local edits; with local edits, saving over
  it is refused until you choose to reload or overwrite.

- **Chat with an agent in `mani web` (third milestone, second half).** "+ Agent" next to the
  message box makes an agent of the folder answer, with its model, tools, policy and limits; it can
  be changed or removed halfway, with a line in the conversation saying so. Tool calls appear in
  order with their results. A tool set to `ask` shows its request in the conversation, with the
  command or path it would act on and three answers: allow once, allow for this conversation, deny.
  "For this conversation" never touches the manifest, and other conversations with the same agent
  still ask. A request survives a reload of the tab, and an unanswered one is denied after 10
  minutes, as in `mani serve`. The editor has "Try in chat".

  Each agent has one runtime, built on its first message and shared by its conversations; saving
  the file replaces it, and its MCP servers close once no reply uses them. Relative paths in the
  manifest (`workspace`, `observability.journal.path`) count from the folder `mani web` manages,
  as if `mani run` started there, and replies land in the agent's journal with `source=web`.

- **New agent in `mani web` (fourth milestone).** "New agent" on the Agents page starts from a
  template, walks through name and model, tools, permissions and limits, shows the file with its
  problems ("Fix" goes back to the step that owns each one), and opens a real conversation with
  the agent before it is saved: tools and approvals work, nothing is written to disk and the
  journal stays off. After saving, "Your agent works" opens it in the chat, in the editor or on
  its triggers. A draft left halfway is kept in the browser and comes back on return. "Import
  YAML" checks a pasted or chosen manifest and saves it into the editor.

- **Four more `mani init` templates:** `docs`, `monitor`, `classifier` and `blank`, next to
  `agent`. `mani init --help` lists them with what each gives you. The templates move to
  `internal/templates`, shared by `mani init` and `mani web`.

- **Custom model endpoints.** A provider can be any server that speaks the OpenAI, Anthropic or
  Ollama API: LM Studio, vLLM, llama.cpp, a company gateway. It lives in `config.json` with a
  `type`, an optional `label`, and an optional `models` list that wins over what the server
  reports, for endpoints that list nothing. Its key is optional and goes in `auth.json`; an
  OpenAI-compatible endpoint without one gets no `Authorization` header. A manifest names it in
  `identity.provider` like any other. `mani web` adds, tests and removes them from the Providers
  page; removing one deletes it from `config.json`.

### Changed

- **`mani validate` lists every problem, each with its line,** instead of stopping at the first:

  ```
  mani validate: agent.yaml: 2 problems
    line 8: tool unknown "teleport" (built-in: bash, delegate, …)
    line 13: limits.max_duration: time: unknown unit " minutes" in duration "2 minutes"
  ```

  Behind it, `manifest.Check(data, baseDir)` returns `[]manifest.Problem` (field, line, message),
  and `Spec.Problems()` collects what `Validate` checks. `Validate` and `Load` still return the
  first problem, with the same wording as before.

- **`app/` is split into packages along its boundaries.** Breaking for library users, at the
  import level only — no behaviour changes:

  | Before | After |
  |---|---|
  | `app.RuntimeSpec`, `app.LoadManifest`, `app.DefaultSpec` | `manifest.Spec`, `manifest.Load`, `manifest.Default` |
  | `app.FlowSpec`, `app.LoadFlow(path)` | `manifest.Flow`, `manifest.LoadFlow(path, set)` |
  | `app.Journal`, `app.RunRecord`, `app.ListFilter`, … | `journal.Journal`, `journal.RunRecord`, `journal.ListFilter`, … |
  | `app.NewInMemoryJournal`, `app.NewJSONLJournal`, `app.NewMultiJournal` | `journal.NewInMemory`, `journal.NewJSONL`, `journal.NewMulti` |
  | `app.SQLiteJournal`, `app.NewSQLiteJournal` | `sqlite.Journal`, `sqlite.New` (in `journal/sqlite`) |
  | `app.TaskQueue`, `app.NewFileQueue`, `app.NewInMemoryQueue` | `scheduler.Queue`, … (in `internal/scheduler`) |

  `app/` goes from about 4,800 lines to 2,600. `StepSpec.ForEach` and `StepSpec.FromAll` are now
  `manifest.Sources`; build one with `manifest.One("step")`.

### Fixed

- OpenAI and Anthropic streaming adapters support calls without a token callback, including
  compaction requests. Transient retries stop after a callback has emitted partial output.

- **One large tool result could break every request after it.** `read` returned a whole file and
  `bash` a whole command output, with no limit: one `ls -R` or one big file filled a 128k-token
  context, and from then on the provider refused each request (`HTTP 400`, context length
  exceeded). Every tool result is now clipped at 32 KiB (`app.DefaultToolOutputLimit`, about 8k
  tokens), keeping its start and its end around a marker that tells the model how much was cut
  and to read less at once. It applies to manifest agents, the TUI and their subagents, and runs
  after `policy.redact`, so a secret is masked before the cut and no half of it survives.
- **Requests that could never succeed were sent three times.** The retry around every provider
  retried any error, so a refused request (a context too long, a wrong key, an unknown model)
  failed three times with growing pauses and came back as `retry: 3 failed attempts`. Only 408,
  409, 429, 5xx and network errors are retried now, as the official OpenAI and Anthropic SDKs
  do; any other status is reported at once, as it is. A `Retry-After` up to 60 seconds is
  honoured, a longer one fails the request at once. Adapters return `*core.ProviderError`, so a
  library caller can read the status too; the message text is unchanged.
- **Images never reached the model.** `mani run --image` and the image attachments of the chat
  were accepted and then dropped twice over: `Runtime.ExecuteIn` did not pass attachments to the
  agent, and none of the provider adapters (Ollama, OpenAI and the ones built on it, Anthropic)
  converted an image block into its API's format. Both are fixed: Ollama gets `images`, OpenAI a
  `image_url` part with a data URL, Anthropic a base64 `image` block.
- **A reloaded session had empty blocks in every message.** The session file reader sized the
  block list and then appended to it, so each message came back with as many `nil` blocks as real
  ones in front. This affected every session reopened in the TUI. Images are now also saved in the
  session file instead of being dropped.
- **A session saved before its first message kept the title "(no title)" forever.**

- **`mani init` did not work.** The flag default was `agent` while the embedded template was
  named `agents.yaml`, so the first command of every tutorial failed with
  `unknown template "agent" (available: agents)`. A test now runs `init` with no arguments and
  loads what it scaffolds.
- An unknown tool in a manifest listed six built-in tools out of ten; the list is now derived
  from the catalog, and a test keeps the two equal.
- The `run` and `validate` command summaries did not mention flows, and `run` said "deamon". The
  scaffolded manifest suggested a trigger of `type: cron`, which does not exist.
- The flow log and `mani validate` printed a command's arguments joined by spaces, so
  `--team "Small appliances"` read as two arguments. An argument with a space is now quoted.

## [0.2.0] - 2026-10-09

The agent stops being something you chat with and becomes something that
processes data: a result that says which run produced it, one agent over a file
of tasks, and a flow that wires manifests into a pipeline. Plus the agent as a
tool for any MCP client.

The flow file is the newest part of the declarative surface, and the part most
likely to move before 1.0: conditional steps, declared parameters and joining
two branches are all deferred, and all of them touch the grammar. Manifests are
not affected — a flow is a separate file.

### Added

- **MCP server mode.** `mani mcp --config agent.yaml` serves a manifest to any MCP
  client over stdio — Claude Desktop, Claude Code, an IDE, another agent. The
  whole agent is one tool: `identity.name` is its name, `identity.description`
  is what the calling model reads, and `output.schema`, when declared, becomes
  the tool's output schema, with the result returned both as structured content
  and as its JSON in text.

  Every call is a fresh run. Permissions are fail-closed, since a client cannot
  answer an `ask`. A failed run, or a missing or malformed `task`, comes back as a
  tool error the calling model can read rather than a protocol error it cannot.
  Policy, limits and the journal apply unchanged, and runs are journaled with
  source `mcp`: an agent called from inside an editor leaves the same audit trail
  as one started by a trigger.

  Under `mani mcp`, `identity.name` is required and must match
  `^[a-zA-Z0-9_-]{1,64}$` — stricter than MCP, because clients pass the name on
  to model APIs that refuse anything else.

  stdout carries only the protocol: an end-to-end test runs the real binary with
  debug logging and fails on any non JSON-RPC line.

- **Flows: a pipeline of manifests.** A file that declares `flow:` is run by the
  same command as an agent — `mani run --config letters.flow.yaml` — and `mani
  run`/`mani validate` tell the two apart by what the file says, not by its
  name. A flow has no behaviour of its own: every step is a manifest or a
  command that already runs by itself.

  ```yaml
  flow: idea_letters
  about: "Rebuilds the timeline of a busta from its transcribed letters"

  steps:
    - step: fetch_letters
      does: "Downloads the transcribed letters of the busta"
      run: python pipeline.py letters --busta ${BUSTA}

    - step: extract_facts
      does: "Reads one letter and extracts sender, recipient, place and date"
      agent: extract.yaml
      for_each: fetch_letters
      jobs: 4

    - step: build_timeline
      does: "Resolves people and places and orders the letters in time"
      run: python pipeline.py timeline
      from_all: extract_facts

  result: build_timeline
  limits: { tokens: 2000000 }
  ```

  The steps run in the order written, and a step may only read the steps above
  it: the graph is acyclic by construction, and the file reads top to bottom.
  `for_each` runs one agent per record, `jobs` at a time; `from_all` runs once
  over every record. A `run:` step is argv with no shell — JSONL records on
  stdin, its own records on stdout — so reshaping data stays the job of a
  script, and a flow needs no template language. `agent:` paths and commands
  resolve against the flow's directory, `${VAR}` expands as in a manifest, and
  the records of the `result:` step are printed on stdout as JSONL, with
  progress on stderr.

  Records are the edges: `{"id": …, "task": …}` plus any fields of your own,
  which travel untouched from step to step — the shelfmark that went in comes
  back out next to the synthesis. An agent receives the upstream `task`, or the
  `result` of the agent before it, as JSON when it is not a string: one agent's
  structured answer is the next one's task, with no glue in between.

  **Resuming is make's rule.** With `--out` every step keeps its records in
  `<out>/<step>/<id>.json`, and a record that exists and is newer than what it
  was made from is not made again. Rerunning a finished flow calls no model at
  all; one more letter costs one extraction. A step that reruns and produces
  identical files does not move their time, so nothing downstream reruns either.
  A record that fails lands in `<out>/<step>/errors.jsonl`, leaves no result
  file, and stops the flow before the next step — so no step ever computes on
  half its input — and the next run retries only what failed. `--limit N` caps
  the new agent runs per step and lets the rest of the flow proceed, which is
  how you try a pipeline on a sample. Without `--out` the flow still runs, in a
  temporary directory it removes at the end.

  `limits.tokens` caps the whole flow across every run, checked before each one
  starts: runs already in flight finish, so the total can pass the cap by at
  most `jobs` runs. The totals are printed at the end either way.

  `mani validate` reads a flow aloud — what each step does, what it reads, in
  which order — so the file can be checked without running it, and `about:` and
  `does:` are required for the same reason.

- **`mani batch`: one agent over a file of tasks.** JSONL in, one record out per
  task:

  ```bash
  mani batch --config classify.yaml --in reviews.jsonl --out out/ --jobs 4
  ```

  A batch is a flow of one step, so it is the same executor and the same rules:
  resume by skipping what is already there, `errors.jsonl` for the failures,
  exit 1 when any task failed, and extras carried through to the result. With
  `--out` each result is its own file; without it they stream to stdout as they
  finish, so a batch fits in a pipe. The whole input is validated before the
  first model call: a malformed line, a missing or repeated `id`, or an `id`
  that cannot be a file name fails immediately rather than two thousand runs
  later. A `task` that is not a string is sent as JSON.

- **Provenance on results.** `mani run --provenance` wraps the result with the
  run that produced it, instead of returning it bare:

  ```json
  {
    "result": {"sentiment": "negative"},
    "run": {"id": "8f2a1c…", "source": "cli", "provider": "ollama",
            "model": "qwen3.5:9b", "manifest": "./classify.yaml",
            "started_at": "…", "ended_at": "…", "in_tokens": 412, "out_tokens": 23}
  }
  ```

  The bare result stays the default, because `output.schema` declares what a run
  returns and wrapping it would make every manifest describe a sub-object. The
  records of a batch or a flow always carry it, under `run`.

- **The journal records the structured result.** `run_end` now carries it, so
  `RunRecord.Results` holds what the agent answered and the audit trail says
  what was decided, not only that something was. It arrives in all three
  adapters at once, with no schema change — the fold over events does the work —
  and `mani runs <id>` shows it.

- **Vocabularies from a file.** An enum can be loaded instead of written out:

  ```yaml
  person: { type: string, enum: !include ./people.txt }
  ```

  One value per line, with blank lines and `#` comments allowed, or a list in a
  `.json`, `.yaml` or `.yml` file — the extension decides. It works anywhere in
  a schema, including the items of an array, a nested object and a subprocess
  tool's own schema, and the path is relative to the manifest, like
  `identity.prompt`. An empty file, a duplicate value or a malformed list is an
  error naming the field, and an absolute path is refused. Above 200 values
  `mani validate` warns on stderr — the schema travels on every call — without
  ever failing the load.


### Changed

- **Structured output is validated in depth.** The validator only looked at the
  top level: an array of objects, or an object inside an object, passed with
  whatever it contained. It now recurses into array items and nested
  properties, checking types, `required` and `enum` at every level, and names
  the offending element — `letters[1].places[1] must be a string`. An answer
  that used to pass now gets the usual retry, so a manifest with a nested
  schema may see one more model call than before, and a result that was never
  the declared shape stops being accepted.

- **`Journal.Finish` takes a `RunOutcome`.** Breaking for anyone implementing
  the port outside the repo: `Finish(runID string, out RunOutcome)` replaces
  `Finish(runID, status string)`, where `RunOutcome` carries the status and the
  result.

- **`tool.PropertySchema.Enum` is a `*tool.EnumValues`.** Breaking for library
  users who build schemas in Go: a plain `[]string` becomes
  `&tool.EnumValues{Values: []string{…}}`. The JSON stays a plain array, so
  nothing changes for providers or MCP clients. The type is what carries a
  deferred `!include`.

- **`mani run` dispatches on the file.** `--in`, `--out` and `--limit` apply to
  a flow, `--task`, `--image`, `--provenance` and `--insecure` to an agent;
  using one on the other is a usage error that names the flag instead of being
  ignored.

### Fixed

- **A second concurrent run could crash the process.** The session store is one
  map shared by every run of a `Runtime`, and nothing guarded it: two runs at
  once — `mani batch --jobs 2`, or two triggers of the same manifest firing
  together — could end in `fatal error: concurrent map writes`. It is now
  locked, and the race detector covers it.

- **`mani run --image` never worked.** The flag was registered after the
  command line was parsed, so every use of it died with `flag provided but not
  defined: -image`.

- **Tool schemas no longer contain `null`.** Unset JSON Schema keywords
  (`items`, `required`, `enum`, `properties`, `description`) were serialised as
  `null`, which no keyword accepts. Providers were unaffected — each adapter
  converts the schema into its own types — but it would have reached MCP clients
  as an invalid output schema. They are now omitted.

## [0.1.5] - 2026-09-07

One feature: the run journal can be a SQLite database instead of a directory of
JSONL files. The port did not change, so nothing else had to.

### Added

- **SQLite journal backend.** `observability.journal.backend: sqlite` stores the
  run history in one indexed database; `path` then names a file instead of a
  directory. Everything else is unchanged: the same `Journal` port, the same
  `mani runs`, the same `GET /runs` and the same filters.

  Listing stops scanning every run. On 200 runs of 25 events, `List(limit=20)`
  takes **0.09 ms** against JSONL's 23.55 ms, because run headers live in their
  own table with an index on `(started_at, run_id)` instead of being recomputed
  from the events. Writes cost about 0.2 ms per event against JSONL's 0.014 —
  the price of a transaction, invisible next to a model call.

  The driver is `modernc.org/sqlite`, pure Go: releases still cross-compile to
  five targets with `CGO_ENABLED=0`. It costs about 5.7 MB of binary, and it is
  linked whether or not you use it.

  Contributed by @mikemikimike.

- **`mani runs --path` accepts either journal shape.** A directory is read as
  JSONL, a file as SQLite. Before, a database path failed with
  `mkdir runs.db: not a directory`.

### Changed

- **`Journal` gained `Close() error`.** Breaking for anyone implementing the
  port outside the repo: `InMemoryJournal` and `JSONLJournal` return nil, the
  SQLite adapter releases its handle. Previously four owners type-asserted for
  an optional `Close`, which is a contract every new caller had to remember.

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

[Unreleased]: https://github.com/Federicoand98/mani/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/Federicoand98/mani/releases/tag/v0.1.5
[0.1.4]: https://github.com/Federicoand98/mani/releases/tag/v0.1.4
[0.1.3]: https://github.com/Federicoand98/mani/releases/tag/v0.1.3
[0.1.2]: https://github.com/Federicoand98/mani/releases/tag/v0.1.2
[0.1.1]: https://github.com/Federicoand98/mani/releases/tag/v0.1.1
