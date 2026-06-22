package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	OllamaBaseURL string `json:"ollama_base_url"`
	UI            string `json:"ui"`
	Thinking      bool   `json:"thinking"`
	Debug         bool   `json:"debug"`
	ContextWindow int    `json:"context_window"`
}

func defaults() Config {
	return Config{
		Provider:      "ollama",
		Model:         "qwen3.5:9b",
		OllamaBaseURL: "http://localhost:11434",
		UI:            "tui",
		Thinking:      true,
		Debug:         true,
		ContextWindow: 8192,
	}
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
	cfg := defaults()
	path := ConfigPath()

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", path, err)
		}

	case errors.Is(err, os.ErrNotExist):
		if werr := Save(cfg); werr != nil {
			fmt.Fprintf(os.Stderr, "config: fail to create %s: %v\n", path, werr)
		}

	default:
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}

	applyEnv(&cfg)
	return cfg, nil
}

func applyEnv(c *Config) {
	if v, ok := os.LookupEnv("MANI_PROVIDER"); ok {
		c.Provider = v
	}

	if v, ok := os.LookupEnv("MANI_MODEL"); ok {
		c.Model = v
	}

	if v, ok := os.LookupEnv("MANI_OLLAMA_BASE_URL"); ok {
		c.OllamaBaseURL = v
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
