// Package config loads mani configuration and credentials.
//
// Settings live in config.json and may be overridden by MANI_* environment variables;
// each provider carries its own base URL and model. Credentials are kept apart in
// auth.json (mode 0600) and are never written to config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Federicoand98/mani/core"
)

type ProviderConfig struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model,omitempty"`
	// Enabled marks a provider the user added (from mani web). Providers have a
	// default base URL whether or not anyone uses them, so presence alone says nothing.
	Enabled bool `json:"enabled,omitempty"`

	// A custom provider (an endpoint mani has no adapter named after) says which
	// API it speaks: "openai", "anthropic" or "ollama". Empty for the built-in ones.
	Type string `json:"type,omitempty"`
	// Label is the name the console shows for a custom provider.
	Label string `json:"label,omitempty"`
	// Models, when set, is the list offered, for endpoints that do not list theirs.
	Models      []string                    `json:"models,omitempty"`
	ModelLimits map[string]core.ModelLimits `json:"model_limits,omitempty"`
}

// Custom reports whether the provider is one the user declared, not one of mani's.
func (p ProviderConfig) Custom() bool { return p.Type != "" }

type Config struct {
	Provider        string                    `json:"provider"`
	Providers       map[string]ProviderConfig `json:"providers"`
	UI              string                    `json:"ui"`
	Thinking        bool                      `json:"thinking"`
	Debug           bool                      `json:"debug"`
	ContextWindow   int                       `json:"context_window"`
	MaxOutputTokens int                       `json:"max_output_tokens"`
	MaxIterations   int                       `json:"max_iterations"`
	LegacyModel     string                    `json:"legacy_model,omitempty"`
	LogLevel        string                    `json:"log_level"` // error | warn | info | debug
}

func defaults() Config {
	return Config{
		Provider: "ollama",
		Providers: map[string]ProviderConfig{
			"ollama":     {BaseURL: "http://localhost:11434", Model: "qwen3.5:9b"},
			"openai":     {BaseURL: "https://api.openai.com/v1"},
			"anthropic":  {BaseURL: "https://api.anthropic.com"},
			"copilot":    {BaseURL: "https://api.githubcopilot.com"},
			"openrouter": {BaseURL: "https://openrouter.ai/api/v1"},
		},
		UI:              "tui",
		Thinking:        true,
		Debug:           true,
		MaxOutputTokens: 4096,
		MaxIterations:   10,
		LogLevel:        "debug",
	}
}

func (c Config) ProviderBaseURL(name string) string {
	if url, ok := c.Providers[name]; ok {
		return url.BaseURL
	}
	return ""
}

func (c Config) ActiveModel() string {
	return c.Providers[c.Provider].Model
}

func (c Config) ProviderModel(name string) string {
	return c.Providers[name].Model
}

func (c *Config) SetActiveModel(model string) {
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}

	pc := c.Providers[c.Provider]
	pc.Model = model
	c.Providers[c.Provider] = pc
}

func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mani")
}

func SessionsDir() string { return filepath.Join(ConfigDir(), "sessions") }
func ConfigPath() string  { return filepath.Join(ConfigDir(), "config.json") }

func Load() (Config, error) {
	cfg, err := loadFile(true)
	if err != nil {
		return cfg, err
	}
	applyEnv(&cfg)
	return cfg, nil
}

// Update reads config.json without the MANI_* overrides, applies fn and writes it
// back, so a value that only lives in the environment never lands in the file.
func Update(fn func(*Config) error) error {
	cfg, err := loadFile(false)
	if err != nil {
		return err
	}
	if err := fn(&cfg); err != nil {
		return err
	}
	return Save(cfg)
}

// loadFile returns the defaults overlaid with config.json; create writes the
// defaults out when the file does not exist yet.
func loadFile(create bool) (Config, error) {
	cfg := defaults()
	path := ConfigPath()

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", path, err)
		}
		if cfg.LegacyModel != "" {
			pc := cfg.Providers[cfg.Provider]
			if pc.Model == "" {
				pc.Model = cfg.LegacyModel
				if cfg.Providers == nil {
					cfg.Providers = map[string]ProviderConfig{}
				}
				cfg.Providers[cfg.Provider] = pc
			}
			cfg.LegacyModel = ""
		}

	case errors.Is(err, os.ErrNotExist):
		if !create {
			break
		}
		if werr := Save(cfg); werr != nil {
			slog.Warn("config: creazione file fallita", "path", path, "err", werr)
		}

	default:
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}

	return cfg, nil
}

func applyEnv(c *Config) {
	if v, ok := os.LookupEnv("MANI_PROVIDER"); ok {
		c.Provider = v
	}

	if v, ok := os.LookupEnv("MANI_MODEL"); ok {
		c.SetActiveModel(v)
	}

	if v, ok := os.LookupEnv("MANI_UI"); ok {
		c.UI = v
	}

	if v, ok := os.LookupEnv("MANI_THINKING"); ok {
		c.Thinking = v == "true"
	}

	if v, ok := os.LookupEnv("MANI_DEBUG"); ok {
		c.Debug = v == "true"
	}

	if v, ok := os.LookupEnv("MANI_CONTEXT_WINDOW"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.ContextWindow = n
			pc := c.Providers[c.Provider]
			if pc.ModelLimits == nil {
				pc.ModelLimits = make(map[string]core.ModelLimits)
			}
			limits := pc.ModelLimits[pc.Model]
			limits.ContextWindow = n
			pc.ModelLimits[pc.Model] = limits
			c.Providers[c.Provider] = pc
		}
	}
	if v, ok := os.LookupEnv("MANI_MAX_OUTPUT_TOKENS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxOutputTokens = n
		}
	}

	if v, ok := os.LookupEnv("MANI_LOG_LEVEL"); ok {
		c.LogLevel = v
	}

	if v, ok := os.LookupEnv("MANI_MAX_ITERATIONS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxIterations = n
		}
	}
}

func Save(c Config) error {
	path := ConfigPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "	")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("config: write: %w", err)
	}

	return os.Rename(tmp, path)
}
