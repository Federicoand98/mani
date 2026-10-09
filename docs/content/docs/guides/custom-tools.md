---
title: Add a tool in any language
description: "Turn a script into a governed tool by declaring it in the manifest: JSON on stdin, JSON on stdout, no plugin SDK."
weight: 5
---

Goal: the agent can call your code — a Python lookup, a shell script, a Go binary — with the same
permissions, risk level and audit trail as a built-in tool.

Prerequisites: an executable that reads stdin and writes stdout.

## The contract

mani writes the tool input as JSON on stdin. Your program writes the result on stdout. A non-zero
exit turns stderr into the error the model reads. That is the whole interface: no SDK, no
generated bindings, no plugin ABI.

{{% steps %}}

### Write the script

```python
#!/usr/bin/env python3
"""Resolves a person's name against a local register."""
import json, sys

REGISTER = {"isabella": "Isabella d'Este, Marchioness of Mantua",
            "francesco": "Francesco II Gonzaga, Marquess of Mantua"}

req = json.load(sys.stdin)
name = req.get("name", "").strip().lower()
if not name:
    print("name is required", file=sys.stderr)
    sys.exit(1)

match = REGISTER.get(name)
if match is None:
    print(f"no match for {name!r}", file=sys.stderr)
    sys.exit(1)

print(match)
```

```bash
chmod +x lookup.py
echo '{"name":"isabella"}' | ./lookup.py
```

```
Isabella d'Este, Marchioness of Mantua
```

Test it like this before you wire it in. A tool that fails from the shell will fail from the
agent, with one more layer in the way.

### Declare it

```yaml
capabilities:
  workspace: .
  tools:
    - read
    - name: lookup_person
      description: "Resolves a person's name against the register. Use it before naming anyone."
      command: ./lookup.py
      risk: none
      schema:
        type: object
        properties:
          name: { type: string, description: "the name as written in the source" }
        required: [name]

policy:
  tools:
    lookup_person: allow
```

The `description` is a prompt. It is what the model reads to decide whether to call the tool, so
write it as an instruction, not as a changelog entry. The `schema` is what the model fills in, and
mani validates the call against it before your script runs.

### Give it the right risk level

```yaml
      risk: none      # none | network | write | execute
```

Be honest here: the level is what governance acts on. A script that writes files is `write`, one
that makes HTTP calls is `network`, one that shells out is `execute`. A tool marked `none` runs
ungated and in parallel, so mislabelling it removes the protection you declared elsewhere.

### Pass secrets without writing them down

```yaml
    - name: deploy
      description: "Triggers a deploy of the named service."
      command: ./deploy.sh
      args: ["--wait"]
      env: { API_TOKEN: ${DEPLOY_TOKEN} }
      risk: execute
```

`env` values go through the same `${VAR}` expansion as the rest of the manifest, so the token
lives in your environment and the manifest stays committable. An unset variable fails the load
rather than passing an empty token.

{{% /steps %}}

## When a tool beats a schema

A closed list of twenty values belongs in an `enum`. A register of ten thousand names does not:
the schema travels with every request, so a long list is a cost on every call. A lookup tool
costs one extra call, and only when the model needs it.

```yaml
# small, closed set: constrain the output
severity: { type: string, enum: [low, medium, high] }

# large register: give it a capability instead
- name: lookup_person
  command: ./lookup.py
```

The difference is what each one does. An `enum` constrains the answer; a tool adds an ability.
`mani validate` warns above 200 enum values for exactly this reason.

## MCP servers

The other way to add tools is to point the manifest at an MCP server, over HTTP or stdio:

```yaml
capabilities:
  mcp:
    - { name: deepwiki, url: https://mcp.deepwiki.com/sse }
    - { name: local, command: ./mcp-server, args: ["--stdio"] }
```

Its tools appear to the agent under the server's name and are governed like any other tool.
Reach for a subprocess tool when you own the code and it is small; reach for MCP when the tool
already exists as a server.

## What you should see

- `mani validate` counts your tool in the `tools:` line and rejects a policy entry that names a
  tool you did not declare.
- The journal records a `tool_call` with the arguments and a `tool_result`, so you can see what
  the model actually sent.
- A failing script surfaces its stderr to the model, which can then retry with different
  arguments rather than dying.

Next: [use it from your editor](../editor-mcp/).
