package app

import (
	"sort"
	"testing"

	"github.com/Federicoand98/mani/manifest"
	"github.com/Federicoand98/mani/tool"
)

func TestBuild_PlanningAndDelegateAreTools(t *testing.T) {
	s := manifest.Default()
	s.Identity.Provider = "ollama"
	s.Capabilities.Tools = []manifest.ToolRef{{Name: "read"}, {Name: "planning"}, {Name: "delegate"}}

	rt, err := Build(t.Context(), s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer rt.Close()

	got := map[string]bool{}
	for _, tl := range rt.tools {
		got[tl.Name()] = true
	}
	for _, want := range []string{"read", "planning", "delegate"} {
		if !got[want] {
			t.Errorf("tool %q non registrato; presenti: %v", want, got)
		}
	}
}

// Senza dichiararli, planning e delegate NON esistono: è il punto di G2.
func TestBuild_NoImplicitTools(t *testing.T) {
	s := manifest.Default()
	s.Identity.Provider = "ollama"
	s.Capabilities.Tools = []manifest.ToolRef{{Name: "read"}}

	rt, err := Build(t.Context(), s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer rt.Close()

	for _, tl := range rt.tools {
		if tl.Name() == "planning" || tl.Name() == "delegate" {
			t.Errorf("tool %q non dichiarato ma registrato", tl.Name())
		}
	}
}

// Un solo vocabolario: la chiave del manifest DEVE essere il nome del tool a runtime.
// Se divergono, `policy.tools` non blocca nulla (cade sul default) e i subagent che
// referenziano il tool falliscono a runtime — entrambi in silenzio.
func TestBuiltinTools_ManifestKeyMatchesRuntimeName(t *testing.T) {
	s := manifest.Default()
	s.Identity.Provider = "ollama"

	builtins := []string{"read", "edit", "write", "bash", "planning", "delegate"}
	for _, name := range builtins {
		s.Capabilities.Tools = append(s.Capabilities.Tools, manifest.ToolRef{Name: name})
	}

	rt, err := Build(t.Context(), s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer rt.Close()

	runtimeNames := map[string]bool{}
	for _, tl := range rt.tools {
		runtimeNames[tl.Name()] = true
	}
	for _, key := range builtins {
		if !runtimeNames[key] {
			t.Errorf("la chiave di manifest %q non corrisponde a nessun tool a runtime; presenti: %v",
				key, runtimeNames)
		}
	}
}

// The manifest package keeps its own list of built-in tool names, so loading a
// manifest needs no runtime. This is the test that stops the two from drifting:
// a name the catalog can build but the manifest rejects is a tool nobody can
// declare, and the reverse is a manifest that validates and then fails to build.
func TestBuiltinTools_ManifestListMatchesTheCatalog(t *testing.T) {
	catalog := make([]string, 0, len(toolContructors))
	for name := range toolContructors {
		catalog = append(catalog, name)
	}
	sort.Strings(catalog)

	declared := manifest.BuiltinTools()
	if len(catalog) != len(declared) {
		t.Fatalf("catalog = %v\nmanifest = %v", catalog, declared)
	}
	for i := range catalog {
		if catalog[i] != declared[i] {
			t.Fatalf("catalog = %v\nmanifest = %v", catalog, declared)
		}
	}
}

// A program that adds a tool to the catalog must be able to declare it in a
// manifest; before the manifest package had its own list, this was implicit.
func TestRegisterToolConstructor_MakesTheNameDeclarable(t *testing.T) {
	const name = "test_registered_tool"
	if manifest.IsBuiltinTool(name) {
		t.Fatalf("%s is already known", name)
	}
	RegisterToolConstructor(name, func(ToolDeps) (tool.Tool, error) { return nil, nil })
	t.Cleanup(func() {
		delete(toolContructors, name)
		manifest.UnregisterBuiltinTool(name)
	})

	if !manifest.IsBuiltinTool(name) {
		t.Errorf("a registered tool is still rejected by the manifest")
	}
}
