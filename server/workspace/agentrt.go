package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/manifest"
)

// buildAgent is app.Build; tests swap it to count builds.
var buildAgent = app.Build

// agentRuntimes keeps one Runtime per manifest (decision M3-9): built on the
// first message, shared by every conversation with that agent, and replaced
// when the file changes. A replaced Runtime is closed (with its MCP servers)
// only once no reply is using it.
type agentRuntimes struct {
	mu sync.Mutex
	m  map[string]*agentRT
}

type agentRT struct {
	rt      *app.Runtime
	version string
	refs    int
	stale   bool
}

// acquire returns the Runtime of the agent at rel and a release func to call
// when the reply ends. ctx bounds the MCP connections, so it is the server's.
func (s *Server) acquireAgent(ctx context.Context, rel string) (*app.Runtime, func(), error) {
	abs, err := s.resolve(rel, true)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("the agent %s does not exist any more: remove it from this conversation, or pick another", rel)
	} else if err != nil {
		return nil, nil, err
	}
	v := version(data)

	a := &s.agents
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m == nil {
		a.m = map[string]*agentRT{}
	}
	if e := a.m[rel]; e != nil {
		if e.version == v {
			e.refs++
			return e.rt, a.releaser(e), nil
		}
		e.stale = true
		if e.refs == 0 {
			e.rt.Close()
		}
		delete(a.m, rel)
	}

	spec, problems := manifest.Check(data, filepath.Dir(abs))
	if len(problems) > 0 {
		return nil, nil, fmt.Errorf("the agent %s has %d problem(s), first: %s; fix it in the editor", rel, len(problems), problems[0].Message)
	}
	s.anchor(&spec)
	rt, err := buildAgent(ctx, spec)
	if err != nil {
		return nil, nil, fmt.Errorf("the agent %s cannot start: %w", rel, err)
	}
	// Conversations with an agent are saved with the others, in the folder.
	if st, err := s.chats.sessions(); err == nil {
		rt.WithSessionStore(st)
	}
	e := &agentRT{rt: rt, version: v, refs: 1}
	a.m[rel] = e
	return rt, a.releaser(e), nil
}

func (a *agentRuntimes) releaser(e *agentRT) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			e.refs--
			if e.stale && e.refs == 0 {
				e.rt.Close()
			}
		})
	}
}

func (a *agentRuntimes) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, e := range a.m {
		e.rt.Close()
		delete(a.m, k)
	}
}

// anchor makes the relative paths of a manifest count from the working folder,
// as if mani run had been started there (decision M3-7).
func (s *Server) anchor(spec *manifest.Spec) {
	at := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(s.opt.Dir, p)
	}
	if spec.Capabilities.Workspace == "" {
		spec.Capabilities.Workspace = s.opt.Dir
	} else {
		spec.Capabilities.Workspace = at(spec.Capabilities.Workspace)
	}
	spec.Observability.Journal.Path = at(spec.Observability.Journal.Path)
}
