package config

import (
	"os"
	"testing"
)

func TestLoad_DefaultsWhenUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isola: nessun config.json reale toccato
	for _, k := range []string{
		"MANI_MODEL", "MANI_OLLAMA_BASE_URL", "MANI_PROVIDER",
		"MANI_UI", "MANI_THINKING", "MANI_DEBUG", "MANI_CONTEXT_WINDOW",
	} {
		os.Unsetenv(k)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load inatteso: %v", err)
	}
	if c.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("default base URL inatteso: %q", c.OllamaBaseURL)
	}
	if c.Model != "qwen3.5:9b" {
		t.Errorf("default model inatteso: %q", c.Model)
	}
}

func TestLoad_EnvOverridesDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MANI_OLLAMA_BASE_URL", "http://remote:9999")
	t.Setenv("MANI_MODEL", "llama3")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load inatteso: %v", err)
	}
	if c.OllamaBaseURL != "http://remote:9999" {
		t.Errorf("override base URL non applicato: %q", c.OllamaBaseURL)
	}
	if c.Model != "llama3" {
		t.Errorf("override model non applicato: %q", c.Model)
	}
}
