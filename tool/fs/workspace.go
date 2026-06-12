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
		return "", fmt.Errorf("edit_file: failed to resolve workspace root: %w", err)
	}

	abs := filepath.Clean(filepath.Join(root, path))

	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("edit_file: access to path '%s' is outside of the workspace", path)
	}

	if abs == root {
		return "", fmt.Errorf("edit_file: path cannot be the workspace root")
	}

	return abs, nil
}
