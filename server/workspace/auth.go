package workspace

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const cookieName = "mani_session"

// sessions implements the one-time link: mani web prints /?t=<token>; the first
// request carrying it gets a session cookie, and the token is replaced so the
// link in the terminal (and in the browser history) cannot be reused.
type sessions struct {
	mu      sync.Mutex
	token   string
	ids     map[string]bool
	onLogin func(next string)
}

func newSessions(token string, onLogin func(string)) *sessions {
	return &sessions{token: token, ids: map[string]bool{}, onLogin: onLogin}
}

// RandomToken returns 32 random bytes, hex encoded.
func RandomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails on supported platforms
	return hex.EncodeToString(b)
}

// exchange spends the token if t matches it and returns a new session id.
func (s *sessions) exchange(t string) (string, bool) {
	s.mu.Lock()
	if s.token == "" || subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) != 1 {
		s.mu.Unlock()
		return "", false
	}
	s.token = RandomToken()
	next := s.token
	id := RandomToken()
	s.ids[id] = true
	s.mu.Unlock()

	if s.onLogin != nil {
		s.onLogin(next)
	}
	return id, true
}

func (s *sessions) valid(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ids[c.Value]
}

func (s *sessions) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.valid(r) {
			writeError(w, http.StatusUnauthorized, "not signed in: open the link printed by mani web")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// login handles ?t= on a page request: on success it sets the cookie and
// redirects to the same path without the token. It reports whether it answered.
func (s *sessions) login(w http.ResponseWriter, r *http.Request) bool {
	t := r.URL.Query().Get("t")
	if t == "" {
		return false
	}
	id, ok := s.exchange(t)
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(usedLinkPage))
		return true
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   30 * 24 * 3600,
	})
	q := r.URL.Query()
	q.Del("t")
	u := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
	return true
}

// hostCheck answers only to loopback names. A page on another site can make the
// browser resolve its own domain to 127.0.0.1 (DNS rebinding); the Host header
// still carries that domain, and is refused here.
func hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsLoopbackHost(r.Host) {
			http.Error(w, "mani web answers only on localhost", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IsLoopbackHost reports whether hostport names this machine: localhost or a
// loopback IP, with or without a port.
func IsLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameOrigin refuses a state-changing request sent by another origin. The
// SameSite cookie already stops most of them; this covers the rest.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host {
					writeError(w, http.StatusForbidden, "cross-origin request refused")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

const usedLinkPage = `<!doctype html><meta charset="utf-8"><title>mani web</title>
<body style="font:16px/1.5 system-ui;max-width:36rem;margin:15vh auto;padding:0 1rem">
<h1 style="font-size:1.4rem">This link has already been used</h1>
<p>Each link printed by <code>mani web</code> works once. Use the newest link in the terminal,
or restart <code>mani web</code> to get one.</p></body>`
