package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_DefaultsWhenUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isola: nessun config.json reale toccato
	for _, k := range []string{
		"MANI_MODEL", "MANI_PROVIDER", "MANI_UI", "MANI_THINKING",
		"MANI_DEBUG", "MANI_CONTEXT_WINDOW", "MANI_MAX_OUTPUT_TOKENS", "MANI_LOG_LEVEL", "MANI_MAX_ITERATIONS",
	} {
		os.Unsetenv(k)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load inatteso: %v", err)
	}
	if c.Provider != "ollama" {
		t.Errorf("provider default inatteso: %q", c.Provider)
	}
	if got := c.ProviderBaseURL("ollama"); got != "http://localhost:11434" {
		t.Errorf("default base URL inatteso: %q", got)
	}
	if got := c.ActiveModel(); got != "qwen3.5:9b" {
		t.Errorf("default model inatteso: %q", got)
	}
}

func TestLoad_EnvOverridesDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MANI_PROVIDER", "ollama")
	t.Setenv("MANI_MODEL", "llama3")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load inatteso: %v", err)
	}
	if c.Provider != "ollama" {
		t.Errorf("override provider non applicato: %q", c.Provider)
	}
	if c.ActiveModel() != "llama3" {
		t.Errorf("override model non applicato: %q", c.ActiveModel())
	}
}

func TestProviderEnabled_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Update(func(c *Config) error {
		pc := c.Providers["openai"]
		pc.Enabled = true
		c.Providers["openai"] = pc
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Providers["openai"].Enabled {
		t.Error("openai not enabled after the round trip")
	}
	if c.Providers["ollama"].Enabled {
		t.Error("ollama enabled without anyone adding it")
	}
	if c.ProviderBaseURL("openai") != "https://api.openai.com/v1" {
		t.Errorf("default base URL lost: %q", c.ProviderBaseURL("openai"))
	}
}

// A config.json written before the field existed reads the same.
func TestProviderEnabled_OldFileReadsAsDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	old := `{"provider":"ollama","providers":{"ollama":{"base_url":"http://box:11434","model":"llama3"}}}`
	if err := os.MkdirAll(dir+"/mani", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/mani/config.json", []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Providers["ollama"].Enabled || c.ProviderBaseURL("ollama") != "http://box:11434" {
		t.Errorf("old file misread: %+v", c.Providers["ollama"])
	}
}

// Update must not persist a value that only comes from the environment.
func TestUpdate_LeavesEnvOverridesOutOfTheFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MANI_MODEL", "from-env")

	if err := Update(func(c *Config) error { return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "from-env") {
		t.Errorf("env override written to config.json:\n%s", data)
	}
}
