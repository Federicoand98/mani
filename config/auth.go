package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type CredentialType string

const (
	CredAPI   CredentialType = "api"
	CredOAuth CredentialType = "oauth"
)

type Credential struct {
	Type      CredentialType `json:"type"`
	Key       string         `json:"key,omitempty"`        // api key
	Refresh   string         `json:"refresh,omitempty"`    // github oauth refresh token (lungo)
	Access    string         `json:"access,omitempty"`     // copilot token (corto 30 min)
	ExpiresAt time.Time      `json:"expires_at,omitempty"` // scandenza access token
}

type Auth struct {
	Providers map[string]Credential `json:"providers"`
}

func AuthPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}

	return filepath.Join(base, "mani", "auth.json")
}

func LoadAuth() (Auth, error) {
	a := Auth{Providers: map[string]Credential{}}
	data, err := os.ReadFile(AuthPath())

	switch {
	case err == nil:
		if err := json.Unmarshal(data, &a); err != nil {
			return a, fmt.Errorf("auth: parse: %w", err)
		}

		if a.Providers == nil {
			a.Providers = map[string]Credential{}
		}
	case errors.Is(err, os.ErrNotExist):

	default:
		return a, fmt.Errorf("auth: read: %w", err)
	}

	return a, nil
}

func SaveAuth(a Auth) error {
	path := AuthPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("auth: mkdir: %w", err)
	}

	data, err := json.MarshalIndent(a, "", "	")
	if err != nil {
		return fmt.Errorf("auth: marshal: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("auth: write: %w", err)
	}

	return os.Rename(tmp, path)
}

func (a Auth) Get(provider string) (Credential, bool) {
	cred, ok := a.Providers[provider]
	return cred, ok
}

func (a Auth) Set(provider string, cred Credential) {
	if a.Providers == nil {
		a.Providers = map[string]Credential{}
	}

	a.Providers[provider] = cred
}

func (a Auth) Delete(provider string) {
	delete(a.Providers, provider)
}
