package workspace

import (
	"testing"

	"github.com/Federicoand98/mani/config"
)

func TestSettingsRequestedOutputRoundTrip(t *testing.T) {
	f := newFixture(t)
	var out Settings
	if code := f.do("GET", "/api/settings", "", &out); code != 200 || out.MaxOutputTokens != 4096 {
		t.Fatalf("defaults: %d, %+v", code, out)
	}
	if code := f.do("PUT", "/api/settings", `{"maxOutputTokens":128}`, &out); code != 200 || out.MaxOutputTokens != 128 {
		t.Fatalf("saved: %d, %+v", code, out)
	}
	cfg, err := config.Load()
	if err != nil || cfg.MaxOutputTokens != 128 {
		t.Fatalf("persistent output: %+v, %v", cfg, err)
	}
	f.do("GET", "/api/settings", "", &out)
	if out.MaxOutputTokens != 128 {
		t.Fatalf("reload: %+v", out)
	}
}

func TestSettingsRefusesInvalidRequestedOutput(t *testing.T) {
	f := newFixture(t)
	var out APIError
	for _, body := range []string{`{}`, `{"maxOutputTokens":0}`, `{"maxOutputTokens":-1}`, `{"maxOutputTokens":1.5}`, `{"maxOutputTokens":1073741825}`, `{"maxOutputTokens":128,"max_output_tokens":999}`} {
		if code := f.do("PUT", "/api/settings", body, &out); code != 400 {
			t.Errorf("%s: %d, %+v", body, code, out)
		}
	}
	cfg, _ := config.Load()
	if cfg.MaxOutputTokens != 4096 {
		t.Fatalf("invalid input changed output: %d", cfg.MaxOutputTokens)
	}
}
