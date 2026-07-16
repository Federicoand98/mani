package server

import (
	"encoding/json"
	"net/http"

	"github.com/Federicoand98/mani/app"
)

type chatResponse struct {
	Output  map[string]any `json:"output,omitempty"`
	Usage   *usageDTO      `json:"usage,omitempty"`
	Error   string         `json:"error,omitempty"`
	IsError bool           `json:"is_error,omitempty"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	id, err := s.mgr.create(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session_id": id})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": s.mgr.list()})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.mgr.remove(id) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	rt, ok := s.mgr.get(r.PathValue("id"))
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	s.runChat(w, r, rt)
}

func (s *Server) handleChatStateless(w http.ResponseWriter, r *http.Request) {
	rt, err := app.Build(r.Context(), s.mgr.spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rt.Close()
	s.runChat(w, r, rt)
}

func (s *Server) runChat(w http.ResponseWriter, r *http.Request, rt *app.Runtime) {
	var body struct {
		Input string `json:"input"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Input == "" {
		http.Error(w, `invalid input. {"input": "..."} requested`, http.StatusBadRequest)
		return
	}

	var usage app.UsagePayload
	for ev := range rt.Execute(app.WithSource(r.Context(), "server"), body.Input) {
		switch ev.Type {
		case app.EventPermissionRequest:
			ev.Payload.(app.PermissionRequestPayload).Respond <- app.Deny
		case app.EventUsage:
			u := ev.Payload.(app.UsagePayload)
			usage.Input += u.Input
			usage.Output += u.Output
		case app.EventError:
			if p, ok := ev.Payload.(app.ErrorPayload); ok {
				writeJSON(w, http.StatusOK, chatResponse{Error: p.Err.Error(), IsError: true})
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Output: rt.StructuredResult(),
		Usage:  &usageDTO{Input: usage.Input, Output: usage.Output},
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
