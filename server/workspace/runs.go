package workspace

import (
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/journal"
	"github.com/Federicoand98/mani/manifest"
)

const (
	runsPage    = 50
	runsPageMax = 200
	// runsRounds bounds how many journal pages one request reads when filters
	// the journals cannot apply (agent, source, blocked) leave a page short.
	runsRounds = 10
)

// journalSrc is one journal of the folder and the agents that write to it:
// several manifests may name the same path.
type journalSrc struct {
	spec   manifest.Spec // anchored; only its journal block is used
	agents []agentInfo
	j      journal.Journal
}

type agentInfo struct {
	AgentRef
	model string
}

// journals finds every journal the agents of this folder write to, keyed by
// backend and path, and the agents that keep none.
func (s *Server) journals() (map[string]*journalSrc, []AgentRef, error) {
	srcs := map[string]*journalSrc{}
	unrecorded := []AgentRef{}
	err := s.eachManifest(func(rel, abs string, data []byte, _ time.Time) {
		spec, ps := manifest.Check(data, filepath.Dir(abs))
		if len(ps) > 0 && ps[0].Path == "" && spec.Identity.Name == "" {
			return // not readable as a manifest
		}
		name := spec.Identity.Name
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		}
		ref := AgentRef{Name: name, Path: rel}
		js := spec.Observability.Journal
		if !js.Enabled || js.Path == "" {
			unrecorded = append(unrecorded, ref)
			return
		}
		s.anchor(&spec)
		backend := strings.ToLower(strings.TrimSpace(spec.Observability.Journal.Backend))
		if backend == "" {
			backend = "jsonl"
		}
		key := backend + ":" + filepath.Clean(spec.Observability.Journal.Path)
		src := srcs[key]
		if src == nil {
			src = &journalSrc{spec: spec}
			srcs[key] = src
		}
		src.agents = append(src.agents, agentInfo{ref, spec.Identity.Provider + " / " + spec.Identity.Model})
	})
	return srcs, unrecorded, err
}

// open opens every journal; one that cannot be opened is left out.
// ponytail: reopened on every request (and so on every poll); keep them open
// in the server if a folder with many SQLite journals makes polling slow.
func openAll(srcs map[string]*journalSrc) (opened []*journalSrc, closeAll func()) {
	for _, src := range srcs {
		if j, ok := app.OpenJournalReader(src.spec); ok {
			src.j = j
			opened = append(opened, src)
		}
	}
	return opened, func() {
		for _, src := range opened {
			_ = src.j.Close()
		}
	}
}

// who is the agent of this folder a run of src belongs to, when it is known.
func (src *journalSrc) who(name string) (agentInfo, bool) {
	if name == "" {
		return agentInfo{}, false
	}
	for _, a := range src.agents {
		if a.Name == name {
			return a, true
		}
	}
	return agentInfo{}, false
}

type tagged struct {
	journal.RunMeta
	src *journalSrc
}

func toRun(m journal.RunMeta, src *journalSrc) Run {
	r := Run{
		ID: m.ID, Agent: m.Agent, Task: m.Task, Source: m.Source, SessionID: m.SessionID, Status: m.Status,
		StartedAt: stamp(m.StartedAt), EndedAt: stamp(m.EndedAt),
		Summary: RunSummary{LLMCalls: m.Summary.LLMCalls, ToolCalls: m.Summary.ToolCalls, InTokens: m.Summary.InTokens,
			OutTokens: m.Summary.OutTokens, Blocked: m.Summary.Blocked, Masked: m.Summary.Masked, Errors: m.Summary.Errors},
	}
	if r.Status == "" {
		r.Status = "running"
	}
	if a, ok := src.who(m.Agent); ok {
		r.AgentPath, r.Model = a.Path, a.model
	}
	return r
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// handleListRuns: GET /api/runs?status=&agent=&source=&session=&period=&blocked=1&before=&limit=
// The journals filter by status and time; agent, source and blocked are
// applied here, reading further pages while one comes back short.
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := runsPage
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = min(n, runsPageMax)
	}
	f := journal.ListFilter{Status: q.Get("status"), SessionID: q.Get("session"), Limit: limit}
	switch q.Get("period") {
	case "24h":
		f.Since = time.Now().Add(-24 * time.Hour)
	case "", "7d":
		f.Since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		f.Since = time.Now().Add(-30 * 24 * time.Hour)
	case "all":
	default:
		writeError(w, http.StatusBadRequest, "period: one of 24h, 7d, 30d, all")
		return
	}
	if b := q.Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before: "+err.Error())
			return
		}
		f.Before = t
	}
	agent, source, blocked := q.Get("agent"), q.Get("source"), q.Get("blocked") == "1"
	keep := func(m journal.RunMeta) bool {
		switch {
		case agent == "unknown" && m.Agent != "", agent != "" && agent != "unknown" && m.Agent != agent:
			return false
		case source != "" && m.Source != source:
			return false
		case blocked && m.Summary.Blocked == 0:
			return false
		}
		return true
	}

	srcs, unrecorded, err := s.journals()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agent != "" && agent != "unknown" {
		for k, src := range srcs {
			if _, ok := src.who(agent); !ok {
				delete(srcs, k)
			}
		}
	}
	opened, closeAll := openAll(srcs)
	defer closeAll()

	out := RunList{Runs: []Run{}, Unrecorded: unrecorded}
	for range runsRounds {
		page, next := listPage(opened, f)
		for _, m := range page {
			if keep(m.RunMeta) {
				out.Runs = append(out.Runs, toRun(m.RunMeta, m.src))
			}
		}
		out.Next = ""
		if !next.IsZero() {
			out.Next = stamp(next)
		}
		if len(out.Runs) >= limit || next.IsZero() {
			break
		}
		f.Before = next
	}
	s.titleConversations(out.Runs)
	writeJSON(w, http.StatusOK, out)
}

// listPage merges one page of every journal, newest first, and cuts it to
// f.Limit. next is the cursor of the following page, zero when there is none.
// Cutting the merge at f.Limit is what keeps the cursor exact: a journal that
// had more than it returned stopped at a run no newer than the last one kept.
func listPage(opened []*journalSrc, f journal.ListFilter) (page []tagged, next time.Time) {
	more := false
	for _, src := range opened {
		metas, err := src.j.List(f)
		if err != nil {
			continue
		}
		if len(metas) >= f.Limit {
			more = true
		}
		for _, m := range metas {
			page = append(page, tagged{m, src})
		}
	}
	sort.Slice(page, func(i, j int) bool {
		if !page[i].StartedAt.Equal(page[j].StartedAt) {
			return page[i].StartedAt.After(page[j].StartedAt)
		}
		return page[i].ID > page[j].ID
	})
	if len(page) > f.Limit {
		page, more = page[:f.Limit], true
	}
	if more && len(page) > 0 {
		next = page[len(page)-1].StartedAt
	}
	return page, next
}

// handleGetRun: GET /api/runs/{id}?after=<seq>, the whole run, or only the
// events after seq for a page that already has the rest.
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	after := -1
	if a := r.URL.Query().Get("after"); a != "" {
		n, err := strconv.Atoi(a)
		if err != nil {
			writeError(w, http.StatusBadRequest, "after: "+err.Error())
			return
		}
		after = n
	}
	srcs, _, err := s.journals()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	opened, closeAll := openAll(srcs)
	defer closeAll()

	for _, src := range opened {
		rec, err := src.j.Get(id)
		if err != nil {
			continue
		}
		d := RunDetail{Run: toRun(rec.Meta(), src), Manifest: rec.Agent.Manifest, Answer: rec.Answer, Error: rec.Error,
			Result: rec.Results, Events: []RunEvent{}}
		if rec.Agent.Provider != "" || rec.Agent.Model != "" {
			d.Model = rec.Agent.Provider + " / " + rec.Agent.Model // what ran, not what the manifest says today
		}
		if l := rec.Limits; l != (journal.Limits{}) {
			d.Limits = &RunLimits{MaxTokens: l.MaxTokens, MaxToolCalls: l.MaxToolCalls, MaxIterations: l.MaxIterations, MaxDuration: l.MaxDuration}
		}
		for _, ev := range rec.Events {
			if ev.Seq > after {
				d.Events = append(d.Events, RunEvent{Seq: ev.Seq, At: stamp(ev.At), Depth: ev.Depth, Kind: string(ev.Kind), Data: ev.Data})
			}
		}
		if rec.Source == "web" && rec.SessionID != "" {
			if st, err := s.chats.sessions(); err == nil {
				if sess, err := st.Load(rec.SessionID); err == nil {
					d.Conversation, d.ConversationTitle, d.Title = sess.ID, sess.Title, sess.Title
				}
			}
		}
		writeJSON(w, http.StatusOK, d)
		return
	}
	writeError(w, http.StatusNotFound, "no run "+id+" in the journals of this folder")
}

// titleConversations gives the runs of console conversations their title.
func (s *Server) titleConversations(runs []Run) {
	titles := map[string]string{}
	for i, r := range runs {
		if r.Source != "web" || r.SessionID == "" {
			continue
		}
		if len(titles) == 0 {
			st, err := s.chats.sessions()
			if err != nil {
				return
			}
			metas, err := st.List()
			if err != nil {
				return
			}
			for _, m := range metas {
				titles[m.ID] = m.Title
			}
			titles[""] = "" // listed once, even when there are none
		}
		runs[i].Title = titles[r.SessionID]
	}
}
