package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler registra se la richiesta è passata oltre il middleware.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthMiddleware_RejectsBadCredentials(t *testing.T) {
	cases := map[string]string{
		"header assente":      "",
		"token errato":        "Bearer wrong",
		"senza schema Bearer": "secret",
		"schema sbagliato":    "Basic secret",
		"token vuoto":         "Bearer ",
		"prefisso del giusto": "Bearer secre",
		"suffisso in eccesso": "Bearer secretx",
		"case diverso":        "bearer secret",
		"spazio finale":       "Bearer secret ",
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			reached := false
			h := authMiddleware("secret", okHandler(&reached))

			req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, atteso 401", rec.Code)
			}
			if reached {
				t.Error("la richiesta non doveva raggiungere l'handler")
			}
		})
	}
}

func TestAuthMiddleware_AcceptsValidToken(t *testing.T) {
	reached := false
	h := authMiddleware("secret", okHandler(&reached))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, atteso 200", rec.Code)
	}
	if !reached {
		t.Error("la richiesta doveva raggiungere l'handler")
	}
}

// Token vuoto = modalità --insecure: il middleware è trasparente.
// È l'unico caso in cui si passa senza credenziali, ed è esplicito nel CLI.
func TestAuthMiddleware_EmptyTokenDisablesAuth(t *testing.T) {
	reached := false
	h := authMiddleware("", okHandler(&reached))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !reached || rec.Code != http.StatusOK {
		t.Errorf("con token vuoto la richiesta deve passare: reached=%v status=%d", reached, rec.Code)
	}
}

// L'handshake WebSocket è una GET HTTP: deve passare dallo stesso gate,
// altrimenti il data plane resterebbe scoperto.
func TestAuthMiddleware_CoversWebSocketUpgrade(t *testing.T) {
	reached := false
	h := authMiddleware("secret", okHandler(&reached))

	req := httptest.NewRequest(http.MethodGet, "/sessions/abc/turn", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("l'upgrade WS senza token deve dare 401, ottenuto %d", rec.Code)
	}
	if reached {
		t.Error("l'upgrade WS non doveva raggiungere l'handler")
	}
}
