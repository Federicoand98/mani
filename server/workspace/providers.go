package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/llm/copilot"
)

type catalogEntry struct {
	label string
	local bool
	auth  string
}

// catalog lists the providers mani can talk to, in the order the console shows them.
var catalog = []struct {
	name string
	catalogEntry
}{
	{"ollama", catalogEntry{"Ollama", true, "none"}},
	{"openai", catalogEntry{"OpenAI", false, "key"}},
	{"anthropic", catalogEntry{"Anthropic", false, "key"}},
	{"openrouter", catalogEntry{"OpenRouter", false, "key"}},
	{"copilot", catalogEntry{"GitHub Copilot", false, "device"}},
}

func lookup(name string) (catalogEntry, bool) {
	for _, c := range catalog {
		if c.name == name {
			return c.catalogEntry, true
		}
	}
	return catalogEntry{}, false
}

// listModels is a variable so tests can stand in for a provider.
var listModels = app.ProviderModels

const testTimeout = 15 * time.Second

func toDTO(name string, e catalogEntry, cfg config.Config, auth config.Auth) Provider {
	pc := cfg.Providers[name]
	p := Provider{Name: name, Label: e.label, Local: e.local, Auth: e.auth, BaseURL: pc.BaseURL, Enabled: pc.Enabled, Model: pc.Model}
	if cred, ok := auth.Get(name); ok && cred.Key != "" {
		p.KeyHint = hint(cred.Key)
	}
	return p
}

func hint(key string) string {
	if len(key) <= 4 {
		return "…"
	}
	return "…" + key[len(key)-4:]
}

func loadBoth() (config.Config, config.Auth, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, config.Auth{}, err
	}
	auth, err := config.LoadAuth()
	return cfg, auth, err
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := ProviderList{Providers: []Provider{}}
	for _, c := range catalog {
		out.Providers = append(out.Providers, toDTO(c.name, c.catalogEntry, cfg, auth))
	}
	writeJSON(w, http.StatusOK, out)
}

// provider resolves {name} or answers 404.
func provider(w http.ResponseWriter, r *http.Request) (string, catalogEntry, bool) {
	name := r.PathValue("name")
	e, ok := lookup(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown provider %q", name))
	}
	return name, e, ok
}

func (s *Server) handlePutProvider(w http.ResponseWriter, r *http.Request) {
	name, e, ok := provider(w, r)
	if !ok {
		return
	}
	var in ProviderInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	auth, err := config.LoadAuth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cred, hasCred := auth.Get(name)
	switch e.auth {
	case "key":
		if in.Key != "" {
			auth.Set(name, config.Credential{Type: config.CredAPI, Key: in.Key})
			if err := config.SaveAuth(auth); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		} else if !hasCred || cred.Key == "" {
			writeError(w, http.StatusBadRequest, e.label+" needs an API key")
			return
		}
	case "device":
		if !hasCred || cred.Refresh == "" {
			writeError(w, http.StatusBadRequest, e.label+" is added by signing in with GitHub, not with a key")
			return
		}
	}

	if err := config.Update(func(c *config.Config) error {
		pc := c.Providers[name]
		pc.Enabled = true
		if in.BaseURL != "" {
			pc.BaseURL = strings.TrimRight(in.BaseURL, "/")
		}
		if in.Model != "" {
			pc.Model = in.Model
		}
		c.Providers[name] = pc
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeProvider(w, name, e)
}

// handleDeleteProvider turns the provider off and forgets its credential: what
// someone pressing Remove expects. The base URL stays, it is only a default.
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	name, e, ok := provider(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := config.Update(func(c *config.Config) error {
		pc := c.Providers[name]
		pc.Enabled = false
		c.Providers[name] = pc
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	auth, err := config.LoadAuth()
	if err == nil {
		if _, had := auth.Get(name); had {
			auth.Delete(name)
			err = config.SaveAuth(auth)
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeProvider(w, name, e)
}

func (s *Server) writeProvider(w http.ResponseWriter, name string, e catalogEntry) {
	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDTO(name, e, cfg, auth))
}

// handleTestProvider tries the provider with the given URL and key, without
// saving them, by asking it for its models.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	name, _, ok := provider(w, r)
	if !ok {
		return
	}
	var in ProviderInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.BaseURL != "" {
		pc := cfg.Providers[name]
		pc.BaseURL = strings.TrimRight(in.BaseURL, "/")
		cfg.Providers[name] = pc
	}
	if in.Key != "" {
		auth.Set(name, config.Credential{Type: config.CredAPI, Key: in.Key})
	}

	ctx, cancel := context.WithTimeout(r.Context(), testTimeout)
	defer cancel()
	models, err := listModels(ctx, cfg, auth, name)
	if err != nil {
		writeJSON(w, http.StatusOK, TestResult{OK: false, Models: []string{}, Error: providerError(err)})
		return
	}
	writeJSON(w, http.StatusOK, TestResult{OK: true, Models: nonNil(models)})
}

func (s *Server) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	name, e, ok := provider(w, r)
	if !ok {
		return
	}
	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !cfg.Providers[name].Enabled {
		writeError(w, http.StatusConflict, e.label+" is not added")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), testTimeout)
	defer cancel()
	models, err := listModels(ctx, cfg, auth, name)
	if err != nil {
		writeError(w, http.StatusBadGateway, providerError(err))
		return
	}
	writeJSON(w, http.StatusOK, ModelList{Models: nonNil(models)})
}

func providerError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("no answer after %s", testTimeout)
	}
	return err.Error()
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// deviceFlow is the GitHub sign-in for Copilot: one at a time, polled by the console.
type deviceFlow struct {
	mu     sync.Mutex
	status string
	err    string
	cancel context.CancelFunc
}

var (
	startDevice = copilot.StartDeviceFlow
	pollDevice  = copilot.PollDeviceFlow
)

func (s *Server) handleDeviceStart(w http.ResponseWriter, r *http.Request) {
	dc, err := startDevice(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	d := &s.device
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel() // a new code replaces the one being waited on
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(dc.ExpiresIn)*time.Second)
	d.status, d.err, d.cancel = "pending", "", cancel
	d.mu.Unlock()

	go func() {
		defer cancel()
		tok, err := pollDevice(ctx, dc)
		if err == nil {
			err = s.saveCopilot(tok)
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		if ctx.Err() == context.Canceled {
			return // replaced by a newer code
		}
		if err != nil {
			d.status, d.err = "error", err.Error()
			return
		}
		d.status = "done"
	}()

	writeJSON(w, http.StatusOK, DeviceStart{UserCode: dc.UserCode, VerificationURI: dc.VerificationURI, ExpiresIn: dc.ExpiresIn})
}

func (s *Server) saveCopilot(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	auth, err := config.LoadAuth()
	if err != nil {
		return err
	}
	auth.Set("copilot", config.Credential{Type: config.CredOAuth, Refresh: token})
	if err := config.SaveAuth(auth); err != nil {
		return err
	}
	return config.Update(func(c *config.Config) error {
		pc := c.Providers["copilot"]
		pc.Enabled = true
		c.Providers["copilot"] = pc
		return nil
	})
}

func (s *Server) handleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	d := &s.device
	d.mu.Lock()
	st := DeviceStatus{Status: d.status, Error: d.err}
	d.mu.Unlock()
	if st.Status == "" {
		st.Status = "idle"
	}
	writeJSON(w, http.StatusOK, st)
}
