---
title: Documentation
description: How to install mani, write a manifest, and run an agent headless, as a service, on a trigger or as a pipeline.
weight: 1
---

mani is a runtime for agents you declare instead of code. You write a manifest, and the runtime
handles the model loop, the tools, the permissions, the limits and the audit trail.

{{< cards >}}
  {{< card link="getting-started/" title="Getting started" subtitle="Install it, then build an agent that returns typed JSON in about five minutes." >}}
  {{< card link="concepts/" title="Concepts" subtitle="The manifest, the agent loop, governance, runs, and how to orchestrate more than one." >}}
  {{< card link="guides/" title="Guides" subtitle="Typed output, triggers, batches, pipelines, custom tools, editors, running as a service." >}}
  {{< card link="reference/" title="Reference" subtitle="The CLI, the manifest, the flow file, the tools, the HTTP API, events and hooks." >}}
  {{< card link="demos/" title="Demos" subtitle="Three recorded sessions: typed output, an unattended agent, a script as a tool." >}}
  {{< card link="troubleshooting/" title="Troubleshooting" subtitle="The errors mani produces, what they mean, and the fix." >}}
{{< /cards >}}

{{< callout type="info" >}}
  mani is version 0.x. Manifest keys, tool names and Go package paths can change between minor
  releases, and the [CHANGELOG](https://github.com/Federicoand98/mani/blob/master/CHANGELOG.md)
  says what changed. Pin a version if you depend on it.
{{< /callout >}}
