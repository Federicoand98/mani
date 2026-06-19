package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	OllamaBaseURL string
	OllamaModel   string
}

func FromEnv() Config {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen3.5:9b"
	}

	return Config{
		OllamaBaseURL: baseURL,
		OllamaModel:   model,
	}
}

func SessionsDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mani", "sessions")
}
