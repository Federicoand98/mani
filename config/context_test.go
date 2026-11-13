package config

import (
	"testing"

	"github.com/Federicoand98/mani/core"
)

func TestContextDefaultsAndModelLimitPersistence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := defaults()
	if cfg.ContextWindow != 0 || cfg.MaxOutputTokens != 4096 {
		t.Fatalf("defaults=%+v", cfg)
	}
	pc := cfg.Providers["ollama"]
	pc.ModelLimits = map[string]core.ModelLimits{pc.Model: {MaxInputTokens: 245000, MaxOutputTokens: 11000}}
	cfg.Providers["ollama"] = pc
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadFile(false)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Providers["ollama"].ModelLimits[pc.Model].MaxInputTokens != 245000 {
		t.Fatal("per-model limits lost")
	}
}

func TestContextEnvironmentIsModelScoped(t *testing.T) {
	t.Setenv("MANI_CONTEXT_WINDOW", "32000")
	t.Setenv("MANI_MAX_OUTPUT_TOKENS", "2048")
	cfg := defaults()
	applyEnv(&cfg)
	got := cfg.Providers[cfg.Provider].ModelLimits[cfg.ActiveModel()]
	if got.ContextWindow != 32000 || cfg.MaxOutputTokens != 2048 {
		t.Fatalf("limits=%v output=%d", got, cfg.MaxOutputTokens)
	}
}
