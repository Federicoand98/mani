package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/internal/templates"
	"github.com/Federicoand98/mani/manifest"
)

// mani init writes every template as a manifest that loads.
func TestInit_EveryTemplate(t *testing.T) {
	for _, name := range templates.Names() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.yaml")
			if res := runMani(t, t.TempDir(), "init", "--template", name, "-o", path); res.code != 0 {
				t.Fatalf("exit %d\n%s", res.code, res.stderr)
			}
			if _, err := manifest.Load(path); err != nil {
				t.Fatalf("Load: %v", err)
			}
		})
	}
	res := runMani(t, t.TempDir(), "init", "--template", "nope")
	if res.code != exitUsage || !strings.Contains(res.stderr, "available: agent, docs, monitor, classifier, blank") {
		t.Errorf("unknown template: %d %s", res.code, res.stderr)
	}
}

func TestUsage_CommandLinesAreAligned(t *testing.T) {
	var b bytes.Buffer
	usage(&b)

	var seen int
	inCommands := false
	for _, line := range strings.Split(b.String(), "\n") {
		if line == "Commands:" {
			inCommands = true
			continue
		}
		if inCommands && line == "" {
			break
		}
		if !inCommands {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		for _, c := range commands {
			if fields[0] == c.name {
				if !strings.HasPrefix(line, "  ") {
					t.Errorf("riga non indentata: %q", line)
				}
				seen++
			}
		}
	}
	if seen != len(commands) {
		t.Errorf("righe trovate %d, comandi %d", seen, len(commands))
	}
}

// `mani init` with no arguments is the first command anyone runs. In 0.2.0 it
// failed with `unknown template "agent" (available: agents)`: the flag default
// and the embedded file disagreed, and nothing checked that they matched.
func TestInit_DefaultTemplateExists(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if err := runInit(context.Background(), nil); err != nil {
		t.Fatalf("mani init: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if err != nil {
		t.Fatalf("init wrote no agent.yaml: %v", err)
	}
	// What it scaffolds must also load, or the first thing a new user runs
	// after `init` fails too.
	path := filepath.Join(dir, "agent.yaml")
	if _, err := manifest.Load(path); err != nil {
		t.Errorf("the scaffolded manifest does not load: %v\n%s", err, body)
	}
}
