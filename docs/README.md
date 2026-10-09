# The documentation site

The site at <https://federicoand98.github.io/mani/> is built from this directory with
[Hugo](https://gohugo.io) and the [Hextra](https://imfing.github.io/hextra/) theme.

Hextra is installed as a **Hugo module**, declared in `docs/go.mod`. That module is separate from
mani's own Go module: `go build ./...` in the repository root does not see it, and the Go
dependencies of mani are unaffected. The alternative, a git submodule, would make every clone need
`git submodule update --init`.

## Run it locally

Needs Hugo **extended** (0.151 or newer) and the Go toolchain, which Hugo uses to fetch the theme.

```bash
cd docs
hugo server
```

Then open <http://localhost:1313/mani/>. Edits reload immediately.

To check what CI will build:

```bash
hugo --gc --minify     # must finish with no warnings
```

## Add a page

Content lives in `content/docs/`, one directory per section:

```
content/
├── _index.md              the home page (layout: hextra-home)
└── docs/
    ├── _index.md          the documentation landing page
    ├── getting-started/   install · first-agent
    ├── concepts/          manifest · agent-loop · governance · runs · orchestration · glossary
    ├── guides/            structured-output · triggers · batch · flows · custom-tools · editor-mcp · service
    ├── reference/         cli · manifest · flow · tools · http-api · events
    ├── demos.md
    ├── troubleshooting.md
    └── contributing.md
```

Each section directory has an `_index.md` with `{{< cards >}}` pointing at its pages.

Every page needs `title`, `description` and `weight` in the front matter. `weight` sets the order
in the sidebar, so file names carry no numeric prefixes.

```markdown
---
title: Run on a schedule
description: Start an agent from a timer or a webhook instead of a terminal.
weight: 3
---
```

House rules, so the pages stay consistent:

- Two levels in the sidebar, no deeper.
- One subject per page. If a page needs two unrelated H2s, it is two pages.
- H2 and H3 in order: the table of contents is built from them.
- End each page with a link to the next logical one.
- Keep examples real. Manifests come from the repository through the `repofile` shortcode:

  ```markdown
  {{< repofile path="flow/reviews.flow.yaml" lang="yaml" >}}
  ```

  The shortcode reads `_examples/<path>` at build time and links to the file on GitHub, so the
  site never holds a second copy that can drift.

Useful Hextra shortcodes: `callout`, `tabs` with `tab`, `steps`, `cards` with `card`,
`filetree`, and fenced ```mermaid blocks for diagrams.

Two shortcodes are ours, both in `layouts/shortcodes/`: `repofile`, above, and `stage`, which the
home page uses for its numbered sequence. `assets/css/custom.css` is the only stylesheet, and it
styles the home page only — every other page is the theme as it ships. There is no custom
JavaScript.

A front-matter value containing `: ` must be quoted, or Hugo refuses the file:

```markdown
description: "Three recordings: typed output, an unattended agent, a script as a tool."
```

## Deploy

`.github/workflows/pages.yml` builds and publishes on every push to `master` that touches `docs/`
or `_examples/`, and can be run by hand from the Actions tab. The `baseURL` comes from the Pages
configuration at build time, so the value in `hugo.yaml` only matters for local builds.

Repository setting: **Settings → Pages → Source: GitHub Actions**.

A documentation fix does not need a release. Nothing here is versioned with mani, and the site
always describes the released version.
