package server

import (
	"context"
	"testing"
	"time"

	"github.com/Federicoand98/mani/app"
)

// newTestConn builds the part of a connection these tests need: the pending
// table and the outbound queue. No websocket, because forwardPermission only
// pushes to the queue the writer drains.
func newTestConn() *conn {
	return &conn{
		outbound: make(chan serverMsg, 8),
		pending:  make(map[string]chan app.Decision),
	}
}

func shortPermissionTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := permissionTimeout
	permissionTimeout = d
	t.Cleanup(func() { permissionTimeout = old })
}

// request forwards one permission request and returns its id and the channel
// the runtime is waiting on.
func request(t *testing.T, cn *conn) (string, chan app.Decision) {
	t.Helper()
	respond := make(chan app.Decision, 1)
	cn.forwardPermission(context.Background(), app.Event{
		Type: app.EventPermissionRequest,
		Payload: app.PermissionRequestPayload{
			ToolName:  "bash",
			RiskLevel: "execute",
			Input:     map[string]any{"command": "ls"},
			Respond:   respond,
		},
	})

	select {
	case msg := <-cn.outbound:
		if msg.Type != "permission_request" {
			t.Fatalf("outbound message = %q, want permission_request", msg.Type)
		}
		dto, ok := msg.Payload.(permissionRequestDTO)
		if !ok {
			t.Fatalf("payload = %T, want permissionRequestDTO", msg.Payload)
		}
		if dto.RequestID == "" {
			t.Fatal("the request carries no id: the client could not answer it")
		}
		return dto.RequestID, respond
	default:
		t.Fatal("nothing was sent to the client")
		return "", nil
	}
}

func waitDecision(t *testing.T, ch chan app.Decision, within time.Duration) app.Decision {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(within):
		t.Fatalf("no decision within %s", within)
		return app.Deny
	}
}

// A client that is connected but silent used to hold the tool call forever: the
// run waited on a human who was not there. The timeout is the "connected but
// mute" case of fail-closed, next to the disconnect that shutdown already
// handled.
func TestForwardPermission_DeniesWhenNobodyAnswers(t *testing.T) {
	shortPermissionTimeout(t, 30*time.Millisecond)
	cn := newTestConn()

	_, respond := request(t, cn)

	if got := waitDecision(t, respond, time.Second); got != app.Deny {
		t.Errorf("decision = %v, want Deny", got)
	}
	cn.mu.Lock()
	left := len(cn.pending)
	cn.mu.Unlock()
	if left != 0 {
		t.Errorf("%d requests still pending: the entry must go with the answer", left)
	}
}

// The timer must be harmless once the client has answered. A second send on
// that channel is the bug this guards: the run would receive a stale deny, or
// the timer goroutine would block on a full channel.
func TestForwardPermission_AnswerWinsAndTheTimerIsHarmless(t *testing.T) {
	shortPermissionTimeout(t, 30*time.Millisecond)
	cn := newTestConn()

	reqID, respond := request(t, cn)
	cn.routeDecision(reqID, "allow_once")

	if got := waitDecision(t, respond, time.Second); got != app.AllowOnce {
		t.Fatalf("decision = %v, want AllowOnce", got)
	}

	time.Sleep(100 * time.Millisecond) // past the timeout
	select {
	case d := <-respond:
		t.Errorf("a second decision arrived after the client answered: %v", d)
	default:
	}
}

// Ids are per request, so an expiring timer must not answer for the request
// that came after it.
func TestForwardPermission_AnExpiredTimerDoesNotTouchAnotherRequest(t *testing.T) {
	shortPermissionTimeout(t, 40*time.Millisecond)
	cn := newTestConn()

	firstID, first := request(t, cn)
	cn.routeDecision(firstID, "allow_once")
	if got := waitDecision(t, first, time.Second); got != app.AllowOnce {
		t.Fatalf("first decision = %v, want AllowOnce", got)
	}

	shortPermissionTimeout(t, time.Hour) // the second request must not expire during the test
	_, second := request(t, cn)

	time.Sleep(120 * time.Millisecond) // the first timer has fired by now
	select {
	case d := <-second:
		t.Errorf("the first timer answered the second request: %v", d)
	default:
	}
	cn.mu.Lock()
	left := len(cn.pending)
	cn.mu.Unlock()
	if left != 1 {
		t.Errorf("pending = %d, want the second request still waiting", left)
	}
}

// A disconnect is the other half of fail-closed: whatever was pending gets
// denied, never left hanging.
func TestShutdown_DeniesEverythingPending(t *testing.T) {
	shortPermissionTimeout(t, time.Hour)
	cn := newTestConn()

	_, a := request(t, cn)
	_, b := request(t, cn)

	cn.shutdown()

	for name, ch := range map[string]chan app.Decision{"first": a, "second": b} {
		if got := waitDecision(t, ch, time.Second); got != app.Deny {
			t.Errorf("%s decision = %v, want Deny", name, got)
		}
	}
	cn.mu.Lock()
	left := len(cn.pending)
	cn.mu.Unlock()
	if left != 0 {
		t.Errorf("%d requests still pending after shutdown", left)
	}
}
