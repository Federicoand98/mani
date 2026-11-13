package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Federicoand98/mani/manifest"
)

// Folders the agent search never enters.
var skipDirs = map[string]bool{".mani": true, ".git": true, "node_modules": true}

const maxManifestBytes = 1 << 20

// resolve turns a relative path from the API into an absolute one inside the
// working directory, or reports why it cannot. Manifests must be .yaml or .yml.
func (s *Server) resolve(rel string, manifestOnly bool) (string, error) {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") {
		return "", fmt.Errorf("%q is not a path inside the working directory", rel)
	}
	clean := path.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%q is outside the working directory", rel)
	}
	for _, part := range strings.Split(clean, "/") {
		if skipDirs[part] {
			return "", fmt.Errorf("%q is inside %s, which mani web does not manage", rel, part)
		}
	}
	if manifestOnly {
		if ext := path.Ext(clean); ext != ".yaml" && ext != ".yml" {
			return "", fmt.Errorf("%q is not a .yaml file", rel)
		}
	}
	abs := filepath.Join(s.opt.Dir, filepath.FromSlash(clean))
	// A symlink must not lead out of the working directory either.
	if real, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		root, _ := filepath.EvalSymlinks(s.opt.Dir)
		if r, err := filepath.Rel(root, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("%q is outside the working directory", rel)
		}
	}
	return abs, nil
}

func version(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// isAgent reports whether a YAML file is an agent manifest: it has a top-level
// identity block and no flow key. A file that does not parse but mentions
// identity at the start of a line is treated as a broken agent, so it shows up
// as invalid instead of vanishing.
func isAgent(data []byte) bool {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		text := "\n" + string(data)
		return strings.Contains(text, "\nidentity:") && !strings.Contains(text, "\nflow:")
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return false
	}
	m := root.Content[0]
	agent := false
	for i := 0; i+1 < len(m.Content); i += 2 {
		switch m.Content[i].Value {
		case "flow":
			return false
		case "identity":
			agent = true
		}
	}
	return agent
}

func toProblems(ps []manifest.Problem) []Problem {
	out := make([]Problem, len(ps))
	for i, p := range ps {
		out[i] = Problem{Path: p.Path, Line: p.Line, Message: p.Message}
	}
	return out
}

func triggerText(t manifest.TriggerSpec) string {
	switch t.Type {
	case "every":
		return "every " + t.Every
	case "daily":
		return "daily " + t.At
	case "webhook":
		return "webhook " + t.HookPath()
	}
	return t.Type
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	out := AgentList{Agents: []AgentSummary{}}
	err := filepath.WalkDir(s.opt.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable folder is skipped, not fatal
		}
		if d.IsDir() {
			if p != s.opt.Dir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxManifestBytes {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || !isAgent(data) {
			return nil
		}
		rel, _ := filepath.Rel(s.opt.Dir, p)
		out.Agents = append(out.Agents, summarize(filepath.ToSlash(rel), data, filepath.Dir(p), info.ModTime()))
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sort.Slice(out.Agents, func(i, j int) bool { return out.Agents[i].Path < out.Agents[j].Path })
	writeJSON(w, http.StatusOK, out)
}

func summarize(rel string, data []byte, dir string, mod time.Time) AgentSummary {
	spec, ps := manifest.Check(data, dir)
	a := AgentSummary{
		Path: rel, Name: spec.Identity.Name, Description: spec.Identity.Description,
		Provider: spec.Identity.Provider, Model: spec.Identity.Model,
		Tools: len(spec.Capabilities.Tools), Triggers: []string{},
		Status: "ok", Problems: len(ps), UpdatedAt: mod.UTC().Format(time.RFC3339),
	}
	for _, t := range spec.Run.Triggers {
		a.Triggers = append(a.Triggers, triggerText(t))
	}
	if len(ps) > 0 {
		a.Status = "problems"
		// A parse failure leaves no Spec: the file is unreadable as a manifest.
		if ps[0].Path == "" && spec.Identity.Name == "" {
			a.Status = "invalid"
		}
	}
	if a.Name == "" {
		a.Name = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	return a
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	abs, err := s.resolve(rel, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "no agent at "+rel)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, ps := manifest.Check(data, filepath.Dir(abs))
	writeJSON(w, http.StatusOK, AgentFile{Path: rel, Text: string(data), Version: version(data), Problems: toProblems(ps)})
}

// handlePutAgent saves an existing manifest. A save carries the version the
// editor loaded; if the file changed on disk since, it is refused with 409
// and the current version, unless Force (M3-6).
func (s *Server) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	abs, err := s.resolve(rel, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var in AgentWrite
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "no agent at "+rel+": it was moved or deleted")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !in.Force && in.Version != version(current) {
		writeJSON(w, http.StatusConflict, AgentFile{Path: rel, Text: string(current), Version: version(current), Problems: []Problem{}})
		return
	}
	s.writeAgent(w, abs, rel, []byte(in.Text), http.StatusOK)
}

// handleCreateAgent writes a new manifest; an existing file is never replaced.
func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	abs, err := s.resolve(rel, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var in AgentWrite
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(abs); err == nil {
		writeError(w, http.StatusConflict, rel+" already exists")
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeAgent(w, abs, rel, []byte(in.Text), http.StatusCreated)
}

// writeAgent writes through a temporary file, so a crash never leaves half a manifest.
func (s *Server) writeAgent(w http.ResponseWriter, abs, rel string, data []byte, code int) {
	if len(data) > maxManifestBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "a manifest is limited to 1 MB")
		return
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(abs); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := abs + ".mani-tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, ps := manifest.Check(data, filepath.Dir(abs))
	writeJSON(w, code, AgentFile{Path: rel, Text: string(data), Version: version(data), Problems: toProblems(ps)})
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	abs, err := s.resolve(rel, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "no agent at "+rel)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !isAgent(data) {
		writeError(w, http.StatusBadRequest, rel+" is not an agent manifest")
		return
	}
	if err := os.Remove(abs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCheckAgent validates text that is not saved, for the editor as you type.
func (s *Server) handleCheckAgent(w http.ResponseWriter, r *http.Request) {
	var in CheckInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	dir := s.opt.Dir
	if in.Path != "" {
		abs, err := s.resolve(in.Path, true)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		dir = filepath.Dir(abs)
	}
	_, ps := manifest.Check([]byte(in.Text), dir)
	writeJSON(w, http.StatusOK, CheckResult{Problems: toProblems(ps)})
}

// handleFolders lists the subfolders of a folder inside the working directory.
func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	abs := s.opt.Dir
	if rel != "" && rel != "." {
		var err error
		if abs, err = s.resolve(rel, false); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		rel = "."
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "no folder at "+rel)
		return
	}
	out := FolderList{Path: rel, Folders: []string{}}
	for _, e := range entries {
		if e.IsDir() && !skipDirs[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
			out.Folders = append(out.Folders, e.Name())
		}
	}
	writeJSON(w, http.StatusOK, out)
}
