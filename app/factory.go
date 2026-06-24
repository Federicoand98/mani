package app

import (
	"fmt"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/llm/openai"
	"github.com/Federicoand98/mani/llm/openrouter"
)

func newLLMClient(cfg config.Config, auth config.Auth) (core.LLMClient, error) {
	provider, model := cfg.Provider, cfg.Model
	base := cfg.ProviderBaseURL(provider)

	var client core.LLMClient
	switch provider {
	case "ollama":
		client = ollama.NewOllamaClient(base, model)

	case "openai":
		cred, ok := auth.Get("openai")
		if !ok || cred.Key == "" {
			return nil, fmt.Errorf("provider openai: no creds, use /login openai before")
		}

		client = openai.New(openai.Config{BaseURL: base, Model: model, AuthFn: openai.StaticKey(cred.Key)})

	case "openrouter":
		cred, ok := auth.Get("openrouter")
		if !ok || cred.Key == "" {
			return nil, fmt.Errorf("provider openrouter: no creds, use /login openrouter before")
		}

		client = openrouter.New(base, model, cred.Key)

	default:
		return nil, fmt.Errorf("provider %s: unknown", provider)
	}

	return NewRetryClient(client, 3, 500*time.Millisecond), nil
}
