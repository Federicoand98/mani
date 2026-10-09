---
title: Demos
description: "Three recorded terminal sessions: typed output in a pipeline, an unattended agent surviving a kill, and a Python script becoming a governed tool."
weight: 5
---

Three recordings, each about half a minute. They were made with
[vhs](https://github.com/charmbracelet/vhs), and the tapes that produced them are in the
repository next to the output, so you can rerun them.

## Typed output in a pipeline

The manifest declares the shape of the answer, the runtime validates it, and the run drops into a
shell pipeline like any other command.

![An agent returning typed JSON, piped into jq](/mani/demo/triage.gif)

Manifest: [`_examples/demo-triage.yaml`](https://github.com/Federicoand98/mani/blob/master/_examples/demo-triage.yaml) ·
tape: [`triage.tape`](https://github.com/Federicoand98/mani/blob/master/_examples/demo/triage.tape)

Related: [get typed JSON back](../guides/structured-output/).

## An unattended agent that survives a kill

No `--task`: the agent starts itself from a trigger, gets `SIGKILL`ed halfway through, and
resumes the same task on restart because the queue is on disk. The journal shows the policy
blocking `rm -rf` on every pass, while nobody was watching.

![A triggered agent killed mid-task, resuming after restart](/mani/demo/unattended.gif)

Manifest: [`_examples/demo-unattended.yaml`](https://github.com/Federicoand98/mani/blob/master/_examples/demo-unattended.yaml) ·
tape: [`unattended.tape`](https://github.com/Federicoand98/mani/blob/master/_examples/demo/unattended.tape)

Related: [run on a schedule or a webhook](../guides/triggers/).

## Eight lines of Python as a governed tool

A script that reads JSON on stdin and writes on stdout becomes a tool with a risk level, a
policy and an audit trail. No plugin SDK, and no Go.

![A Python script declared as a tool and called by the agent](/mani/demo/polyglot.gif)

Manifest: [`_examples/demo-polyglot.yaml`](https://github.com/Federicoand98/mani/blob/master/_examples/demo-polyglot.yaml) ·
tape: [`polyglot.tape`](https://github.com/Federicoand98/mani/blob/master/_examples/demo/polyglot.tape)

Related: [add a tool in any language](../guides/custom-tools/).

## Running them yourself

```bash
git clone https://github.com/Federicoand98/mani.git
cd mani/_examples/demo
vhs triage.tape          # writes triage.gif
```

The tapes use a local Ollama model, so they need no API key. They also double as a smoke test:
if a tape stops producing the same output, something user-visible changed.

Next: [troubleshooting](../troubleshooting/).
