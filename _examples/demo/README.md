# Demo

Three short recordings, each removing a different reason not to try mani. The `.tape`
files are the source: the GIFs are generated from them, so when the CLI changes you
regenerate instead of re-recording.

| Act | Tape | Objection it answers |
|---|---|---|
| 1 | `triage.tape` | *"how would I use this from a program?"* — the agent is a **typed function**: declare `output.schema` and the run pipes into `jq` |
| 2 | `unattended.tape` | *"what if it runs alone and crashes?"* — it **resumes the same task** after a `kill -9`, and the policy still applied while nobody watched |
| 3 | `polyglot.tape` | *"what if I need a tool that doesn't exist?"* — **eight lines of Python** become a governed tool |

Act 1 is the one at the top of the repo README: it shows something a code-first
framework cannot do in a single line.

Act 2 is the one that has no equivalent elsewhere. It is worth watching to the end:
the task id in `queue/running/` before the crash is the *same id* that completes
into `queue/done/` after the restart. The queue is a maildir — a task's state is the
directory it sits in — so the whole mechanism is legible with `ls`.

## Regenerating

Requires [vhs](https://github.com/charmbracelet/vhs) plus `ttyd` and `ffmpeg`, and
credentials for the provider named in the manifests.

```bash
go install github.com/charmbracelet/vhs@latest
go install ./cmd/mani          # mani must be a real binary on PATH, not a shell alias
mani                           # then: /login copilot

# always from the repo ROOT: the tapes cd into this directory themselves
vhs _examples/demo/triage.tape
vhs _examples/demo/unattended.tape
vhs _examples/demo/polyglot.tape
```

The `Require mani` line makes a tape fail immediately if the binary is missing,
instead of recording a GIF full of "command not found". A shell alias does not
count: `Require` looks in `PATH`.

The manifests use `copilot` with `claude-haiku-4.5`, chosen for latency — a run takes
about 2.5s instead of the ~15s a local 9B model needed, which is the difference
between a demo that holds attention and one that does not. The cost is that
regenerating these GIFs needs a working `/login copilot`. Nothing about the demos
depends on that choice: swap `identity.provider` and `identity.model` in the
manifests and everything still works, which is rather the point.

`unattended.tape` writes `queue/` and `runs/` here while recording; both are
gitignored, and the tape wipes them before each take so every recording starts from
the same state.

## Three rules for timing, all learned the hard way

`Wait+Screen@180s /regex/` blocks until the pattern shows up on screen, which beats
guessing how long a model will take. But:

1. **The regex is matched against the whole screen, including the line you just
   typed.** A marker that also appears in the command matches instantly and the wait
   does nothing. Pick something that can only come from the output — `}` from `jq`,
   `queue recovered` from the daemon.
2. **Long log lines wrap, and a marker split across the wrap is not matched.** This
   is why the `--verbose` steps use `Sleep` instead: a tape that times out here fails
   with a bare `recording failed`, which tells you nothing.
3. **Do not wait for the shell prompt.** `Wait+Screen /\$ $/` never matches: the
   screen buffer does not end exactly with `$ `, so the tape times out.

Rule of thumb: `Wait` for the model, `Sleep` for anything local and deterministic —
`jq` over a file has nothing worth waiting for, and a `Wait` there only adds a way to
fail. Sleep values are tuned against a measured run; if you change provider or model,
re-check them.
