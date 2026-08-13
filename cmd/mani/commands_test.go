package main

import (
	"os"
	"path/filepath"
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
