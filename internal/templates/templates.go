// Package templates holds the manifests mani starts new agents from: mani init
// writes one to disk, and the guided creation of mani web edits one into a new
// agent. Each is a commented file, so whoever opens it later sees every block.
package templates

import "embed"

//go:embed *.yaml
var files embed.FS

type Template struct {
	Name  string
	Title string
	About string
	Text  string
}

// catalog fixes the order and the words shown to people.
var catalog = []struct{ name, title, about string }{
	{"agent", "Plain agent", "Reads and writes files in its folder; writing asks first."},
	{"docs", "Documents assistant", "Reads and searches the documents of its folder and answers with sources."},
	{"monitor", "Daily monitor", "Runs read-only checks every morning; rules forbid deleting and sudo."},
	{"classifier", "Classifier", "One text in, one checked JSON answer out. No tools."},
	{"blank", "Blank", "Only a name and a model; every other block is a commented example."},
}

// List returns every template in display order.
func List() []Template {
	out := make([]Template, 0, len(catalog))
	for _, c := range catalog {
		data, _ := files.ReadFile(c.name + ".yaml") // embedded: present by construction
		out = append(out, Template{Name: c.name, Title: c.title, About: c.about, Text: string(data)})
	}
	return out
}

// Get returns one template by name.
func Get(name string) (Template, bool) {
	for _, t := range List() {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}

// Names lists the template names, for flag help and errors.
func Names() []string {
	out := make([]string, len(catalog))
	for i, c := range catalog {
		out[i] = c.name
	}
	return out
}
