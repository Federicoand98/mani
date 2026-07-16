package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Federicoand98/mani/app"
)

type Server struct {
	mgr     *sessionManager
	token   string
	journal app.Journal
}

func New(spec app.RuntimeSpec, token string) *Server {
	j, _ := app.OpenJournalReader(spec)
	return &Server{mgr: newSessionManager(spec), token: token, journal: j}
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /sessions", s.handleCreateSession)
	mux.HandleFunc("GET /sessions", s.handleListSessions)
	mux.HandleFunc("DELETE /sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /sessions/{id}/chat", s.handleChat)
	mux.HandleFunc("POST /chat", s.handleChatStateless)
	mux.HandleFunc("GET /runs", s.handleListRuns)
	mux.HandleFunc("GET /runs/{id}", s.handleGetRun)
	mux.HandleFunc("/sessions/{id}/turn", s.handleTurn) // ws

	srv := &http.Server{Addr: addr, Handler: loggingMiddleware(authMiddleware(s.token, mux))}

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

// loggingMiddleware: logga ogni richiesta HTTP in arrivo, incluso l'upgrade WebSocket
// (una GET con header Upgrade: websocket). Sta fuori da authMiddleware → vedi anche i 401.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws := r.Header.Get("Upgrade") == "websocket"
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr, "ws", ws)
		next.ServeHTTP(w, r)
	})
}
