package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Federicoand98/mani/app"
)

type Server struct {
	mgr   *sessionManager
	token string
}

func New(spec app.RuntimeSpec, token string) *Server {
	return &Server{mgr: newSessionManager(spec), token: token}
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /sessions", s.handleCreateSession)
	mux.HandleFunc("GET /sessions", s.handleListSessions)
	mux.HandleFunc("DELETE /sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /sessions/{id}/chat", s.handleChat)
	mux.HandleFunc("POST /chat", s.handleChatStateless)
	mux.HandleFunc("/sessions/{id}/turn", s.handleTurn) // ws

	srv := &http.Server{Addr: addr, Handler: authMiddleware(s.token, mux)}

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
