// Package workspace serves mani web: the browser console and the API behind it,
// for every agent of one working directory. It is a driving adapter, like
// server/, but where server/ exposes one manifest, this one owns a folder.
package workspace

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
)

type Options struct {
	Dir     string // the working directory, absolute
	Version string
	Assets  fs.FS // the built console; without index.html a placeholder page is served
	Token   string
	// OnLogin is called when the token is spent, with the one that replaces it,
	// so the command can print a link for another browser.
	OnLogin func(next string)
}

type Server struct {
	opt   Options
	built bool
	auth  *sessions

	mu     sync.Mutex // serialises writes to config.json and auth.json
	device deviceFlow
	chats  *chats
}

func New(opt Options) *Server {
	s := &Server{opt: opt, auth: newSessions(opt.Token, opt.OnLogin), chats: newChats(opt.Dir)}
	if opt.Assets != nil {
		if fi, err := fs.Stat(opt.Assets, "index.html"); err == nil && !fi.IsDir() {
			s.built = true
		}
	}
	return s
}

// Handler is the whole server: API, login exchange and static files, behind the
// host check and the security headers.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	s.api(mux, "GET /api/health", s.handleHealth)
	s.api(mux, "GET /api/providers", s.handleListProviders)
	s.api(mux, "PUT /api/providers/{name}", s.handlePutProvider)
	s.api(mux, "DELETE /api/providers/{name}", s.handleDeleteProvider)
	s.api(mux, "POST /api/providers/{name}/test", s.handleTestProvider)
	s.api(mux, "GET /api/providers/{name}/models", s.handleProviderModels)
	s.api(mux, "POST /api/providers/copilot/device", s.handleDeviceStart)
	s.api(mux, "GET /api/providers/copilot/device", s.handleDeviceStatus)
	s.api(mux, "GET /api/conversations", s.handleListConversations)
	s.api(mux, "POST /api/conversations", s.handleCreateConversation)
	s.api(mux, "GET /api/conversations/{id}", s.handleGetConversation)
	s.api(mux, "PATCH /api/conversations/{id}", s.handlePatchConversation)
	s.api(mux, "DELETE /api/conversations/{id}", s.handleDeleteConversation)
	s.api(mux, "GET /api/conversations/{id}/ws", s.handleChatSocket)
	s.api(mux, "/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint: "+r.Method+" "+r.URL.Path)
	})
	mux.HandleFunc("/", s.handleStatic)

	return hostCheck(securityHeaders(mux))
}

// Close stops the replies being written; each saves what it has.
func (s *Server) Close() {
	s.chats.close()
}

// api registers an endpoint that needs a signed-in browser.
func (s *Server) api(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.Handle(pattern, s.auth.require(sameOrigin(h)))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Health{Version: s.opt.Version, Dir: s.opt.Dir})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("workspace: write response", "err", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, APIError{Error: msg})
}

// readJSON decodes a request body, refusing unknown fields so a typo in the
// client fails loudly instead of being ignored.
func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
