---
title: Concepts
description: "The ideas behind mani: agents as configuration, governance in the runtime, and who decides what at run time."
weight: 2
---

An agent, in mani, is a model in a loop with some tools. It receives a task, decides whether to
call a tool, reads the result, and decides again, until it has an answer or it runs out of the
budget you gave it.

What mani adds is that **you declare all of it**. Which model, which tools, which of those tools
need your approval, how many tokens the whole thing may spend, what shape the answer must have,
when it should start on its own. The runtime reads that file and does the rest. Nothing about the
agent lives in code you have to maintain.

That has a consequence worth stating early: because the rules are in the runtime and not in the
caller, they hold the same way everywhere. An agent started from your terminal, from a webhook at
three in the morning, or from your editor through MCP, is governed by the same lines of YAML and
leaves the same record behind.

{{< cards >}}
  {{< card link="manifest/" title="The manifest" subtitle="Eight blocks, one question each, and how to tell where a new setting belongs." >}}
  {{< card link="agent-loop/" title="The agent loop" subtitle="One turn, step by step: where hooks fire, where permissions gate, what reaches the UI." >}}
  {{< card link="governance/" title="Governance" subtitle="Permissions, risk levels, pattern rules and budgets, and why they live in the core." >}}
  {{< card link="runs/" title="Runs and the journal" subtitle="What a run is, what it records, and how a result points back to the run that produced it." >}}
  {{< card link="orchestration/" title="Orchestration" subtitle="One run, subagents, a batch or a flow: the question is always who decides." >}}
  {{< card link="glossary/" title="Glossary" subtitle="Manifest, run, record, step, port, subagent, journal, workspace." >}}
{{< /cards >}}
