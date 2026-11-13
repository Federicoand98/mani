package workspace

import (
	"net/http"

	"github.com/Federicoand98/mani/config"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, Settings{MaxOutputTokens: cfg.MaxOutputTokens})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad settings: "+err.Error())
		return
	}
	if in.MaxOutputTokens <= 0 || in.MaxOutputTokens > 1<<30 {
		writeError(w, http.StatusBadRequest, "maxOutputTokens must be a positive integer up to 1073741824")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := config.Update(func(c *config.Config) error {
		c.MaxOutputTokens = in.MaxOutputTokens
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.agents.invalidate()
	writeJSON(w, http.StatusOK, in)
}
