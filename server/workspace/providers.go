package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/copilot"
)

type catalogEntry struct {
	label  string
	local  bool
	auth   string // "none", "key", "device", or "optional" for a custom endpoint
	custom bool
}

// catalog lists mani's own providers, in the order the console shows them.
// Custom endpoints the user declared follow, from config.json.
var catalog = []struct {
	name string
	catalogEntry
}{
	{"ollama", catalogEntry{label: "Ollama", local: true, auth: "none"}},
	{"openai", catalogEntry{label: "OpenAI", auth: "key"}},
	{"anthropic", catalogEntry{label: "Anthropic", auth: "key"}},
	{"openrouter", catalogEntry{label: "OpenRouter", auth: "key"}},
	{"copilot", catalogEntry{label: "GitHub Copilot", auth: "device"}},
}

var (
	customName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	apiTypes   = []string{"openai", "anthropic", "ollama"}
)

func builtin(name string) (catalogEntry, bool) {
	for _, c := range catalog {
		if c.name == name {
			return c.catalogEntry, true
		}
	}
	return catalogEntry{}, false
}

// lookup finds a provider by name: one of mani's, or a custom one in cfg.
func lookup(name string, cfg config.Config) (catalogEntry, bool) {
	if e, ok := builtin(name); ok {
		return e, true
	}
	pc, ok := cfg.Providers[name]
	if !ok || !pc.Custom() {
		return catalogEntry{}, false
	}
	return customEntry(name, pc), true
}

func customEntry(name string, pc config.ProviderConfig) catalogEntry {
	label := pc.Label
	if label == "" {
		label = name
	}
	auth := "optional"
	if pc.Type == "anthropic" {
		auth = "key"
	}
	return catalogEntry{label: label, local: loopbackURL(pc.BaseURL), auth: auth, custom: true}
}

// loopbackURL: an endpoint on this machine is local, whatever API it speaks.
func loopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && IsLoopbackHost(u.Host)
}

// listModels is a variable so tests can stand in for a provider.
var listModels = app.ProviderModels
var modelLimits = app.ProviderModelLimits

const testTimeout = 15 * time.Second

func toDTO(name string, e catalogEntry, cfg config.Config, auth config.Auth) Provider {
	pc := cfg.Providers[name]
	p := Provider{
		Name: name, Label: e.label, Local: e.local, Auth: e.auth, BaseURL: pc.BaseURL, Enabled: pc.Enabled, Model: pc.Model,
		Custom: e.custom, Type: pc.Type, Models: pc.Models,
		ModelLimits: pc.ModelLimits,
	}
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
	var custom []string
	for name, pc := range cfg.Providers {
		if _, isBuiltin := builtin(name); !isBuiltin && pc.Custom() {
			custom = append(custom, name)
		}
	}
	sort.Strings(custom)
	for _, name := range custom {
		out.Providers = append(out.Providers, toDTO(name, customEntry(name, cfg.Providers[name]), cfg, auth))
	}
	writeJSON(w, http.StatusOK, out)
}

// provider resolves {name} against cfg or answers 404.
func provider(w http.ResponseWriter, r *http.Request, cfg config.Config) (string, catalogEntry, bool) {
	name := r.PathValue("name")
	e, ok := lookup(name, cfg)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown provider %q", name))
	}
	return name, e, ok
}

// checkCustom validates what declares a custom provider: a new one needs a
// name, an API type and an address; a type, when given, must be one mani speaks.
func checkCustom(name string, known bool, in ProviderInput) string {
	if !known {
		if !customName.MatchString(name) {
			return "a provider name uses lowercase letters, digits, - and _ (at most 32), and is what manifests write in identity.provider"
		}
		if in.Type == "" {
			return "a custom provider needs an API type: openai, anthropic or ollama"
		}
		if in.BaseURL == "" {
			return "a custom provider needs the address of its API"
		}
	}
	if in.Type != "" && !slices.Contains(apiTypes, in.Type) {
		return fmt.Sprintf("unknown API type %q: openai, anthropic or ollama", in.Type)
	}
	if in.BaseURL != "" {
		if u, err := url.Parse(in.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Sprintf("%q is not an http(s) address", in.BaseURL)
		}
	}
	return ""
}

// cleanModels trims the declared list and drops empty or repeated names.
func cleanModels(in []string) []string {
	var out []string
	for _, m := range in {
		if m = strings.TrimSpace(m); m != "" && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// handlePutProvider adds or edits a provider. A name mani does not know adds a
// custom endpoint (decision outside M1: requested before M4).
func (s *Server) handlePutProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in ProviderInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	for model, limits := range in.ModelLimits {
		if strings.TrimSpace(model) == "" || strings.TrimSpace(model) != model {
			writeError(w, http.StatusBadRequest, "model limits need a non-empty, trimmed model name")
			return
		}
		if err := checkModelLimits(limits); err != nil {
			writeError(w, http.StatusBadRequest, "model "+model+": "+err.Error())
			return
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	e, known := lookup(name, cfg)
	custom := !known || e.custom
	if custom {
		if msg := checkCustom(name, known, in); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		pc := cfg.Providers[name]
		if in.Type != "" {
			pc.Type = in.Type
		}
		e = customEntry(name, pc)
	} else if in.Type != "" || in.Models != nil || in.Label != "" {
		writeError(w, http.StatusBadRequest, e.label+" is one of mani's providers: type, label and models are only for custom endpoints")
		return
	}

	cred, hasCred := auth.Get(name)
	switch e.auth {
	case "key", "optional":
		if in.Key != "" {
			auth.Set(name, config.Credential{Type: config.CredAPI, Key: in.Key})
			if err := config.SaveAuth(auth); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		} else if e.auth == "key" && (!hasCred || cred.Key == "") {
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
		if in.ModelLimits != nil {
			if pc.ModelLimits == nil {
				pc.ModelLimits = make(map[string]core.ModelLimits)
			}
			for model, limits := range in.ModelLimits {
				pc.ModelLimits[model] = limits
			}
		}
		if custom {
			if in.Type != "" {
				pc.Type = in.Type
			}

			if in.Label != "" {
				pc.Label = in.Label
			}
			if in.Models != nil {
				pc.Models = cleanModels(in.Models)
			}
		}
		c.Providers[name] = pc
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.agents.invalidate()
	s.writeProvider(w, name)
}

func checkModelLimits(l core.ModelLimits) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if l.ContextWindow == 0 && l.MaxInputTokens == 0 && l.MaxOutputTokens == 0 {
		return errors.New("enter at least one positive model limit")
	}
	if l.ContextWindow > 0 && (l.MaxInputTokens > l.ContextWindow || l.MaxOutputTokens >= l.ContextWindow) {
		return errors.New("input must not exceed the context window; output must leave room for input")
	}
	return nil
}

func (s *Server) handleModelLimits(w http.ResponseWriter, r *http.Request) {
	var in ModelLimitsInput
	if err := readJSON(r, &in); err != nil || strings.TrimSpace(in.Model) == "" {
		writeError(w, http.StatusBadRequest, "model limits discovery needs a model name")
		return
	}
	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	name, _, ok := provider(w, r, cfg)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), testTimeout)
	defer cancel()
	limits, err := modelLimits(ctx, cfg, auth, name, strings.TrimSpace(in.Model))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := limits.Validate(); err != nil {
		writeError(w, http.StatusBadGateway, "invalid endpoint limits: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, limits)
}

// handleDeleteProvider turns a provider off and forgets its credential: what
// someone pressing Remove expects. A built-in keeps its base URL, which is only
// a default; a custom endpoint is removed from config.json altogether.
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	name, e, ok := provider(w, r, cfg)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := config.Update(func(c *config.Config) error {
		if e.custom {
			delete(c.Providers, name)
			return nil
		}
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
	if e.custom {
		writeJSON(w, http.StatusOK, toDTO(name, e, config.Config{}, config.Auth{}))
		return
	}
	s.writeProvider(w, name)
}

func (s *Server) writeProvider(w http.ResponseWriter, name string) {
	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	e, _ := lookup(name, cfg)
	writeJSON(w, http.StatusOK, toDTO(name, e, cfg, auth))
}

// handleTestProvider tries a provider with the given settings, without saving
// them, by asking it for its models. A custom endpoint not saved yet can be
// tested too. When it does not list models but some were declared, the test
// passes with a warning and the declared list.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
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
	e, known := lookup(name, cfg)
	if !known || e.custom {
		if msg := checkCustom(name, known, in); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	pc := cfg.Providers[name]
	if in.BaseURL != "" {
		pc.BaseURL = strings.TrimRight(in.BaseURL, "/")
	}
	if in.Type != "" {
		pc.Type = in.Type
	}
	pc.Models = nil // ask the endpoint itself
	cfg.Providers[name] = pc
	if in.Key != "" {
		auth.Set(name, config.Credential{Type: config.CredAPI, Key: in.Key})
	}

	ctx, cancel := context.WithTimeout(r.Context(), testTimeout)
	defer cancel()
	models, err := listModels(ctx, cfg, auth, name)
	declared := cleanModels(in.Models)
	switch {
	case err != nil && len(declared) > 0:
		writeJSON(w, http.StatusOK, TestResult{OK: true, Models: declared,
			Warning: "the endpoint does not list its models (" + providerError(err) + "); your list is used"})
	case err != nil:
		writeJSON(w, http.StatusOK, TestResult{OK: false, Models: []string{}, Error: providerError(err)})
	default:
		writeJSON(w, http.StatusOK, TestResult{OK: true, Models: nonNil(models)})
	}
}

func (s *Server) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	cfg, auth, err := loadBoth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	name, e, ok := provider(w, r, cfg)
	if !ok {
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
