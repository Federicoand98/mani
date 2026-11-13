package templates

import (
	"strings"
	"testing"

	"github.com/Federicoand98/mani/manifest"
)

// Every template is a valid manifest, carries its block comments, and every
// embedded file is in the catalog.
func TestTemplates_AreValidAndCommented(t *testing.T) {
	list := List()
	if len(list) != len(catalog) {
		t.Fatalf("List = %d templates, catalog = %d", len(list), len(catalog))
	}
	for _, tp := range list {
		t.Run(tp.Name, func(t *testing.T) {
			if tp.Text == "" {
				t.Fatal("no file for this template")
			}
			if _, ps := manifest.Check([]byte(tp.Text), t.TempDir()); len(ps) > 0 {
				t.Errorf("problems: %+v", ps)
			}
			if !strings.Contains(tp.Text, "# who thinks?") {
				t.Error("missing the block comments")
			}
		})
	}
	entries, _ := files.ReadDir(".")
	if len(entries) != len(catalog) {
		t.Errorf("%d embedded files, %d in the catalog", len(entries), len(catalog))
	}
}
