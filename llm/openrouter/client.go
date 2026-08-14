// Package openrouter is the LLM adapter for OpenRouter.
//
// It uses the OpenAI wire format against OpenRouters endpoint.
package openrouter

import "github.com/Federicoand98/mani/llm/openai"

func New(baseURL string, model, key string) *openai.Client {
	return openai.New(openai.Config{
		BaseURL: baseURL,
		Model:   model,
		AuthFn:  openai.StaticKey(key),
		Headers: map[string]string{
			"HTTP-Referer": "https://github.com/Federicoand98/mani",
			"X-Title":      "mani",
		},
	})
}
