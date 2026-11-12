package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/llm/anthropic"
	"github.com/Federicoand98/mani/llm/copilot"
	"github.com/Federicoand98/mani/llm/ollama"
	"github.com/Federicoand98/mani/llm/openai"
	"github.com/Federicoand98/mani/llm/openrouter"
)

type unavailableClient struct{ err error }

func (c unavailableClient) Send(ctx context.Context, messages []core.Message, tools []core.ToolDefinition, tokenHandler core.TokenHandler) (core.LLMResponse, error) {
	return core.LLMResponse{}, c.err
}

var _ core.LLMClient = unavailableClient{}

func newLLMClient(cfg config.Config, auth config.Auth) (core.LLMClient, error) {
	provider, model := cfg.Provider, cfg.ActiveModel()
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

	case "copilot":
		cred, ok := auth.Get("copilot")
		if !ok || cred.Refresh == "" {
			return nil, fmt.Errorf("provider copilot: no creds, use /login copilot before")
		}
		client = copilot.New(base, model, cred)

	case "openrouter":
		cred, ok := auth.Get("openrouter")
		if !ok || cred.Key == "" {
			return nil, fmt.Errorf("provider openrouter: no creds, use /login openrouter before")
		}

		client = openrouter.New(base, model, cred.Key)

	case "anthropic":
		cred, ok := auth.Get("anthropic")
		if !ok || cred.Key == "" {
			return nil, fmt.Errorf("provider anthropic: no creds, use /login anthropic before")
		}

		client = anthropic.New(anthropic.Config{BaseURL: base, Model: model, APIKey: cred.Key})

	default:
		// return nil, fmt.Errorf("provider %s: unknown", provider)
		if base == "" {
			return nil, fmt.Errorf("provider %s: no base url in config", provider)
		}
		cred, ok := auth.Get(provider)
		if !ok || cred.Key == "" {
			return nil, fmt.Errorf("provider %s: no creds, use /login %s before", provider, provider)
		}
		client = openai.New(openai.Config{BaseURL: base, Model: model, AuthFn: openai.StaticKey(cred.Key)})
	}

	return NewRetryClient(client, 3, 500*time.Millisecond), nil
}

// ProviderModels builds a client for provider from cfg and auth and asks it for
// the models it offers. It doubles as a connection test: an unreachable URL or a
// rejected key comes back as the error, worded by the provider.
func ProviderModels(ctx context.Context, cfg config.Config, auth config.Auth, provider string) ([]string, error) {
	cfg.Provider = provider
	client, err := newLLMClient(cfg, auth)
	if err != nil {
		return nil, err
	}
	ml, ok := client.(core.ModelLister)
	if !ok {
		return nil, fmt.Errorf("provider %s: cannot list models", provider)
	}
	return ml.ListModels(ctx)
}
