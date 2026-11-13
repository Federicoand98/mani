package workspace

import (
	"net/http"

	"github.com/Federicoand98/mani/internal/templates"
	"github.com/Federicoand98/mani/manifest"
	"github.com/Federicoand98/mani/session"
)

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	out := TemplateList{Templates: []Template{}}
	for _, t := range templates.List() {
		out.Templates = append(out.Templates, Template{Name: t.Name, Title: t.Title, About: t.About, Text: t.Text})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateDraft starts a trial conversation with a manifest that is not
// saved yet (M4-4). It lives in memory only: its Runtime keeps no journal, and
// its messages never reach .mani/sessions. Folder places the manifest, so its
// relative paths and !include resolve as they will once saved.
func (s *Server) handleCreateDraft(w http.ResponseWriter, r *http.Request) {
	var in DraftInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	dir := s.opt.Dir
	if in.Folder != "" && in.Folder != "." {
		abs, err := s.resolve(in.Folder, false)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		dir = abs
	}
	spec, problems := manifest.Check([]byte(in.Text), dir)
	if len(problems) > 0 {
		writeJSON(w, http.StatusBadRequest, CheckResult{Problems: toProblems(problems)})
		return
	}
	s.anchor(&spec)
	spec.Observability.Journal.Enabled = false

	rt, err := buildAgent(s.chats.ctx, spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the agent cannot start: "+err.Error())
		return
	}
	store := session.NewInMemoryStore()
	rt.WithSessionStore(store)
	sess := session.New(spec.Identity.Model)
	sess.Provider = spec.Identity.Provider
	_ = store.Save(sess)

	c := s.chats
	c.mu.Lock()
	c.drafts[sess.ID] = &draft{rt: rt, store: store}
	c.mu.Unlock()
	writeJSON(w, http.StatusCreated, Draft{ID: sess.ID})
}

// draftOf finds {id} among the trials, or answers 404.
func (s *Server) draftOf(w http.ResponseWriter, r *http.Request) (string, *draft, bool) {
	id := r.PathValue("id")
	c := s.chats
	c.mu.Lock()
	d := c.drafts[id]
	c.mu.Unlock()
	if d == nil {
		writeError(w, http.StatusNotFound, "no such trial: it ended, or mani web restarted")
	}
	return id, d, d != nil
}

func (s *Server) handleDraftSocket(w http.ResponseWriter, r *http.Request) {
	id, d, ok := s.draftOf(w, r)
	if !ok {
		return
	}
	sess, err := d.store.Load(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such trial")
		return
	}
	s.serveSocket(w, r, sess, d.store)
}

// handleDeleteDraft ends a trial: its reply stops, its Runtime closes.
func (s *Server) handleDeleteDraft(w http.ResponseWriter, r *http.Request) {
	id, d, ok := s.draftOf(w, r)
	if !ok {
		return
	}
	c := s.chats
	c.cancel(id)
	c.mu.Lock()
	delete(c.drafts, id)
	delete(c.allowed, id)
	c.mu.Unlock()
	d.rt.Close()
	w.WriteHeader(http.StatusNoContent)
}
