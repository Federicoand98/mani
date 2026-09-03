package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/core"
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

// ---------------------------------------------------------------------------
// BuildDaemon: how a webhook trigger resolves its token
// ---------------------------------------------------------------------------

func webhookSpecFor(t *testing.T, triggers []TriggerSpec, opts ...DaemonOption) (*Daemon, error) {
	t.Helper()
	rt := testRuntime(t, core.NewMock(core.RespText("ok")))
	spec := DefaultSpec()
	spec.Identity.Provider = "ollama"
	spec.Run.Triggers = triggers
	return BuildDaemon(rt, spec, opts...)
}

func TestBuildDaemon_WebhookToken(t *testing.T) {
	hook := func(path, token string) TriggerSpec {
		return TriggerSpec{Type: "webhook", Addr: "127.0.0.1:8099", Path: path, Token: token, Prompt: "x"}
	}

	t.Run("the manifest token wins", func(t *testing.T) {
		t.Setenv("MANI_WEBHOOK_TOKEN", "from-env")
		d, err := webhookSpecFor(t, []TriggerSpec{hook("/deploy", "from-manifest")})
		if err != nil {
			t.Fatalf("BuildDaemon: %v", err)
		}
		if got := d.webhooks[0].token; got != "from-manifest" {
			t.Errorf("token = %q, want the manifest one", got)
		}
	})

	// A manifest written before 0.1.4 has no token: the env var must keep working
	// so the release does not break webhook auth for the second time in a row.
	t.Run("no manifest token falls back to the env var", func(t *testing.T) {
		t.Setenv("MANI_WEBHOOK_TOKEN", "from-env")
		d, err := webhookSpecFor(t, []TriggerSpec{hook("", "")})
		if err != nil {
			t.Fatalf("BuildDaemon: %v", err)
		}
		if got := d.webhooks[0].token; got != "from-env" {
			t.Errorf("token = %q, want the env one", got)
		}
		if got := d.webhooks[0].path; got != "/hook" {
			t.Errorf("path = %q, want the /hook default", got)
		}
	})

	t.Run("each route keeps its own token", func(t *testing.T) {
		t.Setenv("MANI_WEBHOOK_TOKEN", "from-env")
		d, err := webhookSpecFor(t, []TriggerSpec{hook("/deploy", "tok-deploy"), hook("/alert", "tok-alert")})
		if err != nil {
			t.Fatalf("BuildDaemon: %v", err)
		}
		if len(d.webhooks) != 2 {
			t.Fatalf("got %d webhooks, want 2", len(d.webhooks))
		}
		if d.webhooks[0].token == d.webhooks[1].token {
			t.Error("the two routes share a token")
		}
	})

	// Fail-closed: no token anywhere and no --insecure means the daemon refuses
	// to start, rather than serving an unauthenticated hook.
	t.Run("no token anywhere refuses to build", func(t *testing.T) {
		t.Setenv("MANI_WEBHOOK_TOKEN", "")
		_, err := webhookSpecFor(t, []TriggerSpec{hook("/deploy", "")})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "insecure") {
			t.Errorf("error %q should point at --insecure", err)
		}
	})

	t.Run("--insecure allows an unauthenticated hook", func(t *testing.T) {
		t.Setenv("MANI_WEBHOOK_TOKEN", "")
		d, err := webhookSpecFor(t, []TriggerSpec{hook("/deploy", "")}, AllowInsecureWebhook())
		if err != nil {
			t.Fatalf("BuildDaemon: %v", err)
		}
		if d.webhooks[0].token != "" {
			t.Error("token should stay empty")
		}
	})

	// One trigger without a token must not be rescued by another that has one:
	// the check is per route.
	t.Run("one route without a token fails the whole build", func(t *testing.T) {
		t.Setenv("MANI_WEBHOOK_TOKEN", "")
		_, err := webhookSpecFor(t, []TriggerSpec{hook("/deploy", "tok"), hook("/alert", "")})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	// The trigger id reaches the queue, so the journal can tell which hook fired.
	t.Run("the trigger id is carried into the spec", func(t *testing.T) {
		t.Setenv("MANI_WEBHOOK_TOKEN", "tok")
		trig := hook("/deploy", "")
		trig.Name = "deploy-hook"
		d, err := webhookSpecFor(t, []TriggerSpec{trig})
		if err != nil {
			t.Fatalf("BuildDaemon: %v", err)
		}
		if got := d.webhooks[0].id; got != "deploy-hook" {
			t.Errorf("id = %q, want the declared name", got)
		}
	})
}
