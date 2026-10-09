---
title: Guides
description: "Task-shaped walkthroughs: typed output, triggers, batches, pipelines, custom tools, editor integration and running mani as a service."
weight: 3
---

Each guide has one goal, states its prerequisites, and ends with what you should see.

{{< cards >}}
  {{< card link="structured-output/" title="Get typed JSON back" subtitle="Declare the shape of the answer and use a run as a command in a pipeline." >}}
  {{< card link="triggers/" title="Run on a schedule or a webhook" subtitle="Start an agent from a timer or an HTTP call, and survive a restart." >}}
  {{< card link="batch/" title="Process a file of tasks" subtitle="One agent over a JSONL file, in parallel, resumable, with failures kept apart." >}}
  {{< card link="flows/" title="Build a pipeline" subtitle="Chain manifests and scripts into a flow that resumes like make." >}}
  {{< card link="custom-tools/" title="Add a tool in any language" subtitle="A script that reads JSON on stdin becomes a governed tool." >}}
  {{< card link="editor-mcp/" title="Use it from your editor" subtitle="Serve a manifest over MCP so any client can call the agent as one tool." >}}
  {{< card link="service/" title="Run it as a service" subtitle="HTTP and WebSocket, bearer auth, streaming turns and approvals over the wire." >}}
{{< /cards >}}
