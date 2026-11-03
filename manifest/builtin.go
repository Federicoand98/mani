package manifest

import (
	"sort"
	"sync"
)

// The names a manifest may use for a tool without declaring a command. They
// are data here, not a lookup into the runtime's catalog, so that loading a
// manifest needs nothing but this package; app/tool_catalog.go builds the
// tools, and a test there keeps the two lists equal.
var (
	builtinMu sync.RWMutex
	builtin   = map[string]bool{
		"read": true, "edit": true, "write": true, "delete": true,
		"glob": true, "grep": true, "bash": true, "fetch": true,
		"planning": true, "delegate": true,
	}
)

// IsBuiltinTool reports whether name is a tool the runtime can build by name.
func IsBuiltinTool(name string) bool {
	builtinMu.RLock()
	defer builtinMu.RUnlock()
	return builtin[name]
}

// BuiltinTools returns the built-in names, sorted: the list error messages show.
func BuiltinTools() []string {
	builtinMu.RLock()
	defer builtinMu.RUnlock()
	names := make([]string, 0, len(builtin))
	for n := range builtin {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// RegisterBuiltinTool makes name valid in a manifest. app.RegisterToolConstructor
// calls it, so a tool a program adds to the catalog is also one its manifests
// may name.
func RegisterBuiltinTool(name string) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	builtin[name] = true
}

// UnregisterBuiltinTool undoes RegisterBuiltinTool: for a program that removes a
// tool it added, and for the tests that add one.
func UnregisterBuiltinTool(name string) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	delete(builtin, name)
}
