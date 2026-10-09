---
title: mani
description: A declarative agent runtime in Go. One YAML file says which model thinks, which tools it may use, what it is allowed to do and what it must return.
layout: hextra-home
---

<div class="home">
<section class="home-hero">
  <h1>Agents you declare, not agents you code</h1>
  <p class="home-lede">One YAML file says which model thinks, which tools it may use, what it is
  allowed to do and what it must return. mani runs it headless, as a service, on a trigger or
  inside your editor, and writes down what it did.</p>
  <div class="home-actions">
    <a class="home-cta" href="docs/getting-started/">Get started</a>
    <a class="home-link" href="https://github.com/Federicoand98/mani">Source</a>
    <a class="home-link" href="docs/reference/manifest/">Manifest reference</a>
  </div>
  <p class="home-meta">Go 1.25 · Apache 2.0 · one binary · five providers · no runtime dependencies</p>
</section>

{{% stage num="01" title="Declare the agent" %}}
Eight blocks, each answering one question: who thinks, what it can do, what it sees, what it
returns, what it may do, how much it may spend, when it starts, what it leaves behind. Only
`identity` is required, and an unknown key is an error with its line number.

```yaml
identity:
  provider: anthropic
  model: claude-sonnet-5
  prompt: "You triage incoming bug reports."

capabilities:
  tools: [read, grep]

policy:
  tools: { default: deny, read: allow, grep: allow }
  rules:
    - { tool: read, pattern: '\.env$', action: deny, label: "secrets" }

output:
  schema:
    type: object
    properties:
      severity: { type: string, enum: [low, medium, high] }
      component: { type: string }
    required: [severity, component]

limits:
  max_tokens: 20000
  max_duration: 2m
```
{{% /stage %}}

{{% stage num="02" title="Check it before it runs" %}}
`validate` resolves what the runtime resolves: included prompt files, `${VAR}` references, tool
names, policy rules, journal settings. It calls no model, so it belongs in CI.

```bash
mani validate --config triage.yaml
```

```
triage.yaml: ok
  identity:     triage (anthropic / claude-sonnet-5)
  tools:        2
  subagents:    0
  triggers:     0
  output:       structured
```
{{% /stage %}}

{{% stage num="03" title="Run the same file five ways" %}}
The transport is a command, not a rewrite. Policy, limits and the journal come along every time,
because they live in the runtime rather than in the caller.

```bash
mani run --config triage.yaml --task "$(cat report.txt)"   # one task, prints JSON
mani run --config triage.yaml                              # the manifest's triggers, in the foreground
mani serve --config triage.yaml --addr :9000               # HTTP and WebSocket
mani mcp --config triage.yaml                              # a tool inside your editor
mani batch --config triage.yaml --in reports.jsonl --jobs 4 # a file of tasks, resumable
```

With a schema declared, a run is a command you can pipe:

```bash
mani run --config triage.yaml --task "$(cat report.txt)" | jq -r .severity
```

```
high
```

When one agent is not enough, a flow file chains manifests into a pipeline and resumes the way
`make` does: a record newer than what it was made from is not made again.
{{% /stage %}}

{{% stage num="04" title="Read what it did" %}}
Every run leaves a record: the tools it called, the arguments, the results, the permissions it was
denied, the tokens it spent, the answer it gave. Stored as JSONL files or in SQLite, and readable
without a server.

```bash
mani runs --config triage.yaml --status error --since 24h
```

```
ID            STATUS  STARTED              DURATION  TOKENS   TOOLS  BLOCKED
82fdb6ffaa1b  error   2026-08-31 18:06:10  3.6s      681/96   2      1
```

`mani runs <id>` replays one run as a timeline, including the rule that blocked a call. That record
is what makes an unattended agent reviewable the next morning.
{{% /stage %}}

<section class="home-scope">
  <p><strong>What mani is not.</strong> No retrieval pipeline, no vector store, no evaluation
  harness, no prompt-tuning helpers. Those are well served elsewhere, and each one would cost the
  legibility that is the point of this project.</p>
  <p>A feature belongs here if it deepens one of three things: how much of an agent can be
  declared, how much an unattended agent can do without becoming dangerous, or how easy it is to
  run and observe.</p>
</section>

<section class="home-hero">
  <div class="home-actions">
    <a class="home-cta" href="docs/getting-started/install/">Install</a>
    <a class="home-link" href="docs/getting-started/first-agent/">Your first agent</a>
    <a class="home-link" href="docs/reference/cli/">CLI reference</a>
  </div>
  <p class="home-meta">mani is 0.x: manifest keys and package paths can change between minor releases.</p>
</section>
</div>
