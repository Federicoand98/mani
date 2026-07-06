package server

import (
	"context"
	"net/http"
	"sync"

	"github.com/Federicoand98/mani/app"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// due goroutine per connessione: la write pump drena il canale Event verso la WS, la read loop riceve permission_response / cancel

type turnConn struct {
	c       *websocket.Conn
	mu      sync.Mutex
	pending map[string]chan app.Decision
}

func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request) {
	rt, ok := s.mgr.get(r.PathValue("id"))
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()

	ctx := r.Context()
	conn := &turnConn{c: c, pending: make(map[string]chan app.Decision)}

	// 1. primo frame: input del turno
	var first clientMsg
	if err := wsjson.Read(ctx, c, &first); err != nil {
		return
	}

	if first.Type != "input" || first.Input == "" {
		_ = wsjson.Write(ctx, c, serverMsg{Type: "error", Payload: errorDTO{Message: "first message must be: {type:input}"}})
		return
	}

	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 2. read loop: risposte ai permessi - bloccato su <-Respond
	go conn.readLoop(turnCtx, rt, cancel)

	// 3. write pump: eventi -> ws
	for ev := range rt.Execute(turnCtx, first.Input) {
		if ev.Type == app.EventPermissionRequest {
			conn.forwardPermission(ctx, ev)
			continue
		}
		msg, ok := toServerMsg(ev)
		if !ok {
			continue
		}

		if err := wsjson.Write(ctx, c, msg); err != nil {
			cancel()
			return
		}
	}

	_ = c.Close(websocket.StatusNormalClosure, "")
}

// forwardPermission: salva il canale Respond sotto un req_id e manda req al client
func (conn *turnConn) forwardPermission(ctx context.Context, ev app.Event) {
	p := ev.Payload.(app.PermissionRequestPayload)
	reqID := newID()

	conn.mu.Lock()
	conn.pending[reqID] = p.Respond
	conn.mu.Unlock()

	_ = wsjson.Write(ctx, conn.c, serverMsg{
		Type: "permission_request",
		Payload: permissionRequestDTO{
			RequestID: reqID,
			ToolName:  p.ToolName,
			RiskLevel: p.RiskLevel,
			Input:     p.Input,
			Preview:   p.Preview,
		},
	})
}

func (conn *turnConn) readLoop(ctx context.Context, rt *app.Runtime, cancel context.CancelFunc) {
	for {
		var msg clientMsg
		if err := wsjson.Read(ctx, conn.c, &msg); err != nil {
			conn.drainPending()
			cancel()
			return
		}

		switch msg.Type {
		case "permission_response":
			conn.mu.Lock()
			ch, ok := conn.pending[msg.RequestID]
			delete(conn.pending, msg.RequestID)
			conn.mu.Unlock()
			if ok {
				ch <- parseDecision(msg.Decision)
			}
		case "cancel":
			rt.Cancel()
		}
	}
}

func (conn *turnConn) drainPending() {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	for id, ch := range conn.pending {
		ch <- app.Deny
		delete(conn.pending, id)
	}
}
