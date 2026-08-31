package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/app"
)

func TestTemplates_AllParseAndValidate(t *testing.T) {
	for _, name := range templateNames() {
		t.Run(name, func(t *testing.T) {
			data, err := templatesFS.ReadFile("templates/" + name + ".yaml")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "agent.yaml")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			spec, err := app.LoadManifest(path)
			if err != nil {
				t.Fatalf("LoadManifest: %v", err)
			}
			if err := spec.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
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
