package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// adapter su disco

type FileStore struct {
	dir string
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	return &FileStore{dir: dir}, nil
}

func (f *FileStore) path(id string) string {
	return filepath.Join(f.dir, id+".json")
}

func (f *FileStore) Save(s *Session) error {
	s.touch()
	data, err := json.MarshalIndent(toSessionDTO(s), "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	tmp := f.path(s.ID) + ".tmp" // scrittura atomica, prima scrivo tmp poi rinomino
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return os.Rename(tmp, f.path(s.ID))
}

func (f *FileStore) Load(id string) (*Session, error) {
	data, err := os.ReadFile(f.path(id))
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var dto sessionDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return dto.toSession(), nil
}

func (f *FileStore) List() ([]Meta, error) {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var out []Meta
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		id := e.Name()[:len(e.Name())-len(".json")]
		s, err := f.Load(id)
		if err != nil {
			continue
		}

		out = append(out, Meta{ID: s.ID, Title: s.Title, Model: s.Model, UpdatedAt: s.UpdatedAt})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (f *FileStore) Delete(id string) error {
	return os.Remove(f.path(id))
}
