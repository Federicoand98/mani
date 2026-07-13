package config

import (
	"os"
	"testing"
)

func TestLoad_DefaultsWhenUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isola: nessun config.json reale toccato
	for _, k := range []string{
		"MANI_MODEL", "MANI_PROVIDER", "MANI_UI", "MANI_THINKING",
		"MANI_DEBUG", "MANI_CONTEXT_WINDOW", "MANI_LOG_LEVEL", "MANI_MAX_ITERATIONS",
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
