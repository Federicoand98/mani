package app

import (
	"context"
	"strings"
	"testing"
)

func TestBuild_FailsOnUnusableProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	spec := DefaultSpec()
	spec.Identity.Provider = "anthropic"
	spec.Identity.Model = "claude-sonnet-4"

	_, err := Build(context.Background(), spec)
	if err == nil {
		t.Fatal("expected Build to fail without credentials, got nil")
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Fatalf("error should name the provider, got: %v", err)
	}
}
