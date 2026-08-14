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

type conn struct {
	c        *websocket.Conn
	rt       *app.Runtime
	outbound chan serverMsg
	mu       sync.Mutex
	pending  map[string]chan app.Decision
	running  bool
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

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cn := &conn{
		c:        c,
		rt:       rt,
		outbound: make(chan serverMsg, 32),
		pending:  make(map[string]chan app.Decision),
	}

	go cn.writer(ctx) // unico writer
	cn.reader(ctx)    // blocca finchè vive una connessione, ritorna la disconnect
	cn.shutdown()     // drena i permessi
}

func (cn *conn) writer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-cn.outbound:
			if err := wsjson.Write(ctx, cn.c, msg); err != nil {
				return
			}
		}
	}
}

// send: push non bloccante versio il writer
func (cn *conn) send(ctx context.Context, m serverMsg) {
	select {
	case cn.outbound <- m:
	case <-ctx.Done():
	}
}

func (cn *conn) reader(ctx context.Context) {
	for {
		var msg clientMsg
		if err := wsjson.Read(ctx, cn.c, &msg); err != nil {
			return
		}

		switch msg.Type {
		case "input":
			cn.startTurn(ctx, msg.Input)
		case "permission_response":
			cn.routeDecision(msg.RequestID, msg.Decision)
		case "cancel":
			cn.rt.Cancel()
		}
	}
}

func (cn *conn) startTurn(ctx context.Context, input string) {
	if input == "" {
		cn.send(ctx, errMsg("input vuoto"))
		return
	}

	cn.mu.Lock()

	if cn.running {
		cn.mu.Unlock()
		cn.send(ctx, errMsg("turn in progress"))
		return
	}

	cn.running = true
	cn.mu.Unlock()

	go cn.runTurn(ctx, input)
}

func (cn *conn) runTurn(ctx context.Context, input string) {
	defer func() {
		cn.mu.Lock()
		cn.running = false
		cn.mu.Unlock()
	}()

	for ev := range cn.rt.Execute(app.WithSource(ctx, "server"), input) {
		if ev.Type == app.EventPermissionRequest {
			cn.forwardPermission(ctx, ev)
			continue
		}

		if ev.Type == app.EventDone {
			if p, ok := ev.Payload.(app.DonePayload); ok && p.Result != nil {
				cn.send(ctx, serverMsg{Type: "structured", Payload: p.Result})
			}
		}

		if m, ok := toServerMsg(ev); ok {
			cn.send(ctx, m)
		}
	}
}

// forwardPermission: salva il canale Respond sotto un req_id e manda req al client
func (conn *conn) forwardPermission(ctx context.Context, ev app.Event) {
	p := ev.Payload.(app.PermissionRequestPayload)
	reqID := newID()

	conn.mu.Lock()
	conn.pending[reqID] = p.Respond
	conn.mu.Unlock()

	conn.send(ctx, serverMsg{Type: "permission_request", Payload: permissionRequestDTO{
		RequestID: reqID, ToolName: p.ToolName, RiskLevel: p.RiskLevel,
		Input: p.Input, Preview: p.Preview,
	}})
}

func (cn *conn) routeDecision(reqID, decision string) {
	cn.mu.Lock()
	ch, ok := cn.pending[reqID]
	delete(cn.pending, reqID)
	cn.mu.Unlock()
	if ok {
		ch <- parseDecision(decision)
	}
}

func (cn *conn) shutdown() {
	cn.mu.Lock()
	defer cn.mu.Unlock()

	for id, ch := range cn.pending {
		select {
		case ch <- app.Deny:
		default:
		}
		delete(cn.pending, id)
	}
}

func errMsg(s string) serverMsg {
	return serverMsg{
		Type:    "error",
		Payload: errorDTO{Message: s},
	}
}
