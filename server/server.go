package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Federicoand98/mani/app"
)

type Server struct {
	mgr *sessionManager
}

func New(spec app.RuntimeSpec) *Server {
	return &Server{mgr: newSessionManager(spec)}
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /sessions", s.handleCreateSession)
	mux.HandleFunc("GET /sessions", s.handleListSessions)
	mux.HandleFunc("DELETE /sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("/sessions/{id}/turn", s.handleTurn) // ws

	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	slog.Info("mani agent server listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}
