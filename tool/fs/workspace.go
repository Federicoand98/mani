// Package fs provides the filesystem tools: read, edit, write and delete.
//
// Every path is resolved against a workspace root and rejected if it escapes it,
// so an agent cannot touch files outside the directory it was given.
package fs

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Workspace struct {
	root string
}

func NewWorkspace(root string) Workspace {
	return Workspace{root: root}
}

func (w Workspace) Resolve(path string) (string, error) {
	root, err := filepath.Abs(w.root)
	if err != nil {
		return "", fmt.Errorf("fs: failed to resolve workspace root: %w", err)
	}

	abs := filepath.Clean(filepath.Join(root, path))

	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("fs: access to path '%s' is outside of the workspace", path)
	}

	if abs == root {
		return "", fmt.Errorf("fs: path cannot be the workspace root")
	}

	return abs, nil
}
