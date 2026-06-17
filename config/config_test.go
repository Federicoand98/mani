package config

import "testing"

func TestFromEnv_DefaultsWhenUnset(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("OLLAMA_MODEL", "")

	c := FromEnv()
	if c.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("default base URL inatteso: %q", c.OllamaBaseURL)
	}
	if c.OllamaModel != "qwen3.5:9b" {
		t.Errorf("default model inatteso: %q", c.OllamaModel)
	}
}

func TestFromEnv_OverrideWhenSet(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://remote:9999")
	t.Setenv("OLLAMA_MODEL", "llama3")

	c := FromEnv()
	if c.OllamaBaseURL != "http://remote:9999" {
		t.Errorf("override base URL non applicato: %q", c.OllamaBaseURL)
	}
	if c.OllamaModel != "llama3" {
		t.Errorf("override model non applicato: %q", c.OllamaModel)
	}
}
