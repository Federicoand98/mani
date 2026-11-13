package workspace

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/config"
)

// An OpenAI-compatible server on this machine, as LM Studio or vLLM would be.
func fakeOpenAI(t *testing.T, listsModels bool) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" && listsModels {
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen-coder"},{"id":"llama-70b"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

func TestCustomProvider_AddListUseRemove(t *testing.T) {
	f := newFixture(t)
	base := fakeOpenAI(t, true)

	var res TestResult
	f.do("POST", "/api/providers/lmstudio/test", `{"type":"openai","baseUrl":"`+base+`"}`, &res)
	if !res.OK || len(res.Models) != 2 {
		t.Fatalf("test before saving: %+v", res)
	}

	var p Provider
	if code := f.do("PUT", "/api/providers/lmstudio", `{"type":"openai","label":"LM Studio","baseUrl":"`+base+`/"}`, &p); code != 200 {
		t.Fatalf("add: %d", code)
	}
	if !p.Custom || !p.Enabled || p.Label != "LM Studio" || p.Type != "openai" || !p.Local || p.BaseURL != base || p.Auth != "optional" {
		t.Errorf("added = %+v", p)
	}

	var list ProviderList
	f.do("GET", "/api/providers", "", &list)
	if last := list.Providers[len(list.Providers)-1]; last.Name != "lmstudio" || len(list.Providers) != 6 {
		t.Errorf("list = %+v", list.Providers)
	}
	var ml ModelList
	if code := f.do("GET", "/api/providers/lmstudio/models", "", &ml); code != 200 || len(ml.Models) != 2 {
		t.Errorf("models: %d %+v", code, ml)
	}

	// mani core sees it as any other provider.
	cfg, _ := config.Load()
	if pc := cfg.Providers["lmstudio"]; pc.Type != "openai" || !pc.Custom() {
		t.Errorf("config.json = %+v", pc)
	}

	var removed Provider
	if code := f.do("DELETE", "/api/providers/lmstudio", "", &removed); code != 200 || removed.Enabled {
		t.Errorf("remove: %d %+v", code, removed)
	}
	cfg, _ = config.Load()
	if _, still := cfg.Providers["lmstudio"]; still {
		t.Error("a removed custom provider stays in config.json")
	}
}

func TestCustomProvider_DeclaredModelsForAnEndpointThatListsNone(t *testing.T) {
	f := newFixture(t)
	base := fakeOpenAI(t, false)

	var res TestResult
	f.do("POST", "/api/providers/vllm/test", `{"type":"openai","baseUrl":"`+base+`"}`, &res)
	if res.OK {
		t.Errorf("no models and none declared must fail: %+v", res)
	}
	f.do("POST", "/api/providers/vllm/test", `{"type":"openai","baseUrl":"`+base+`","models":["mistral-7b"," ","mistral-7b"]}`, &res)
	if !res.OK || len(res.Models) != 1 || !strings.Contains(res.Warning, "does not list its models") {
		t.Errorf("declared models: %+v", res)
	}

	var p Provider
	f.do("PUT", "/api/providers/vllm", `{"type":"openai","baseUrl":"`+base+`","key":"sk-vllm","models":["mistral-7b"]}`, &p)
	if len(p.Models) != 1 || p.KeyHint != "…vllm" {
		t.Errorf("saved = %+v", p)
	}
	var ml ModelList
	if code := f.do("GET", "/api/providers/vllm/models", "", &ml); code != 200 || len(ml.Models) != 1 || ml.Models[0] != "mistral-7b" {
		t.Errorf("models from the declared list: %d %+v", code, ml)
	}
}

func TestCustomProvider_Refusals(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name, body, want string
	}{
		{"Bad Name", `{"type":"openai","baseUrl":"http://x"}`, "lowercase letters"},
		{"proxy", `{"type":"grpc","baseUrl":"http://x"}`, "unknown API type"},
		{"proxy", `{"type":"openai"}`, "address of its API"},
		{"proxy", `{"type":"openai","baseUrl":"ftp://x"}`, "not an http(s) address"},
		{"claude", `{"type":"anthropic","baseUrl":"http://x"}`, "needs an API key"},
	}
	for _, c := range cases {
		var e APIError
		if code := f.do("PUT", "/api/providers/"+strings.ReplaceAll(c.name, " ", "%20"), c.body, &e); code != 400 || !strings.Contains(e.Error, c.want) {
			t.Errorf("%s %s: %d %q, want %q", c.name, c.body, code, e.Error, c.want)
		}
	}
	var e APIError
	if code := f.do("PUT", "/api/providers/openai", `{"key":"sk-1","type":"anthropic"}`, &e); code != 400 || !strings.Contains(e.Error, "only for custom") {
		t.Errorf("type on a built-in: %d %q", code, e.Error)
	}
}
