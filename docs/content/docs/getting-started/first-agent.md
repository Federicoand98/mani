---
title: Your first agent
description: Scaffold a manifest, run one task headlessly, then turn the answer into typed JSON you can pipe into other tools.
weight: 2
---

Goal: an agent that reads a file, answers with a JSON object, and leaves a record of what it did.

Prerequisites: `mani` on your `PATH` and a model it can reach. See [Install](../install/).

## Scaffold a manifest

{{% steps %}}

### Create the file

```bash
mani init
```

```
created agent.yaml

next:
  mani validate --config agent.yaml
  mani run --config agent.yaml --task "hello"
```

The file it writes is commented, and only the first block is required:

```yaml
identity:                    # who thinks?
  name: my-agent
  description: A helpful assistant
  provider: ollama
  model: qwen3.5:9b
  prompt: |
    You are a helpful assistant. Be concise.

capabilities:                # what can it do?
  workspace: .
  tools:
    - name: read
    - name: write

policy:                      # what is it allowed to do?
  tools:
    write: ask               # allow | ask | deny

limits:                      # how much may it consume?
  max_iterations: 20
  tool_timeout: 60s
```

Three things worth knowing before you run it:

- `workspace` confines every filesystem tool. The agent cannot read or write outside it, whatever
  the model asks for.
- `policy.tools.write: ask` means mani stops and asks you before each write. In the chat you get
  a prompt; headless there is nobody to ask, so the call is denied.
- An unknown key is an error with its line number, so a typo never silently disables a rule.

### Check it

```bash
mani validate --config agent.yaml
```

```
agent.yaml: ok
  identity:     my-agent (ollama / qwen3.5:9b)
  tools:        2
  subagents:    0
  triggers:     0
```

`validate` resolves everything the runtime would resolve: included prompt files, `${VAR}`
references, tool names, policy rules. It makes no model call, which makes it the right thing to
run in CI.

### Run one task

```bash
mani run --config agent.yaml --task "Summarize README.md in two sentences."
```

The answer goes to stdout. Progress and logs go to stderr, so a run fits in a pipeline without
anything extra on the channel you are reading.

{{% /steps %}}

## Ask for JSON instead of prose

Prose is hard to use from a script. Declare the shape you want, and mani will make the model
answer in it. Replace the commented `output` block with:

```yaml
output:
  schema:
    type: object
    properties:
      summary: { type: string }
      severity: { type: string, enum: [low, medium, high] }
    required: [summary, severity]
```

```bash
mani validate --config agent.yaml
```

```
agent.yaml: ok
  identity:     my-agent (ollama / qwen3.5:9b)
  tools:        2
  subagents:    0
  triggers:     0
  output:       structured
```

Now the run prints an object, and only an object:

```bash
mani run --config agent.yaml --task "Read incident.log and classify it." | jq -r .severity
```

```
high
```

Under the hood the agent gets one extra tool, `respond`, and the run ends when it calls it. mani
validates the payload against your schema, including array items and nested objects. An answer
that does not fit is handed back to the model with the reason, and the model tries again. What
reaches stdout always matches what you declared.

{{< callout type="info" >}}
  Use small, closed sets in a schema. `enum: [low, medium, high]` gets you three values you can
  branch on; a free-text field gets you something to parse.
{{< /callout >}}

## Keep a record of the run

Add a journal:

```yaml
observability:
  journal:
    enabled: true
    path: ./runs
```

Run the agent again, then look at what happened:

```bash
mani runs --config agent.yaml
```

```
ID            STATUS  STARTED              DURATION  TOKENS   TOOLS  BLOCKED
82fdb6ffaa1b  ok      2026-08-31 18:06:10  3.6s      681/96   2      1
```

A unique id prefix is enough to open one run, the way `git` works:

```bash
mani runs --config agent.yaml 82fdb6
```

You get the run as a timeline: each tool call with its arguments, each result, each permission
that was denied, the tokens spent, and the answer it returned. That record is what makes an
unattended run reviewable the next morning.

## Where to go next

- [CLI reference](../../reference/cli/) for every command and flag.
- [Manifest reference](../../reference/manifest/) for every block and key.
