package fetch

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/tool"
)

var _ tool.Tool = (*FetchTool)(nil)

// newTestTool builds the tool with an almost-real IP guard: it blocks everything
// blockedIP blocks except loopback, because httptest listens on 127.0.0.1. The
// real guard is exercised separately, in TestSafeClient_RefusesLoopbackAtDial.
func newTestTool(opts ...Option) *FetchTool {
	base := WithIPGuard(func(ip net.IP) bool { return !ip.IsLoopback() && blockedIP(ip) })
	return New(append([]Option{base}, opts...)...)
}

// serve starts a test server answering with the given content type and body.
func serve(t *testing.T, contentType, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// Tool declaration
//
// The risk level is not a detail: policy.tools and policy.network filter on it.
// Downgrading fetch to RiskNone would make it run in parallel and ungated, so
// the value is pinned by a test.
// ---------------------------------------------------------------------------

func TestFetch_Declaration(t *testing.T) {
	ft := New()

	if got := ft.Name(); got != "fetch" {
		t.Errorf("Name = %q, want %q", got, "fetch")
	}
	if got := ft.RiskLevel(); got != core.RiskNetwork {
		t.Errorf("RiskLevel = %v, want %v", got, core.RiskNetwork)
	}

	s := ft.Schema()
	if s.Name != "fetch" {
		t.Errorf("Schema.Name = %q", s.Name)
	}
	if s.Description == "" {
		t.Error("Schema.Description is empty")
	}
	if s.InputSchema.Type != "object" {
		t.Errorf("InputSchema.Type = %q, want %q", s.InputSchema.Type, "object")
	}
	if _, ok := s.InputSchema.Properties["url"]; !ok {
		t.Error("the schema does not declare the 'url' property")
	}
	if len(s.InputSchema.Required) != 1 || s.InputSchema.Required[0] != "url" {
		t.Errorf("Required = %v, want [url]", s.InputSchema.Required)
	}
}

// ---------------------------------------------------------------------------
// Input validation: what the tool refuses without touching the network
// ---------------------------------------------------------------------------

func TestFetch_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"missing url", map[string]any{}, "missing required input"},
		{"empty url", map[string]any{"url": ""}, "missing required input"},
		{"url is not a string", map[string]any{"url": 42}, "missing required input"},
		{"unparsable url", map[string]any{"url": "://nope"}, "invalid url"},
		{"file scheme", map[string]any{"url": "file:///etc/passwd"}, "unsupported scheme: file"},
		{"ftp scheme", map[string]any{"url": "ftp://example.invalid/x"}, "unsupported scheme: ftp"},
		{"gopher scheme", map[string]any{"url": "gopher://example.invalid"}, "unsupported scheme: gopher"},
		// A bare host is an error, not an invitation to guess http on the
		// model's behalf.
		{"no scheme at all", map[string]any{"url": "example.invalid/x"}, "unsupported scheme"},
	}

	ft := New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ft.Execute(context.Background(), tc.input)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// blockedIP: the addresses an agent must not be able to reach.
//
// This is a security control, not an ordinary function: statement coverage is
// not enough, the cases are. A single wrong address here turns `fetch` into an
// exfiltration channel into the internal network.
// ---------------------------------------------------------------------------

func TestBlockedIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v4, rest of the /8", "127.255.255.254", true},
		{"loopback v6", "::1", true},
		{"loopback v4 mapped into v6", "::ffff:127.0.0.1", true},
		{"private 10/8", "10.0.0.1", true},
		{"private 172.16/12", "172.16.5.4", true},
		{"private 192.168/16", "192.168.1.1", true},
		{"unique local v6 (fc00::/7)", "fd00::1", true},
		// 169.254.169.254 is the metadata endpoint on AWS, GCP and Azure: it is
		// the single address an SSRF aims at first.
		{"link-local v4, cloud metadata", "169.254.169.254", true},
		{"link-local v6", "fe80::1", true},
		{"link-local multicast v4", "224.0.0.1", true},
		{"link-local multicast v6", "ff02::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},

		{"public v4", "8.8.8.8", false},
		{"public v4, ordinary unicast", "93.184.216.34", false},
		{"public v6", "2001:4860:4860::8888", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("invalid test address: %q", tc.ip)
			}
			if got := blockedIP(ip); got != tc.blocked {
				t.Errorf("blockedIP(%s) = %v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The guard acts at dial time, before the request leaves
// ---------------------------------------------------------------------------

// httptest listens on 127.0.0.1, which the real guard blocks. This is therefore
// the only test using blockedIP as it ships: every other one goes through
// newTestTool, which lets loopback alone through.
func TestSafeClient_RefusesLoopbackAtDial(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
		w.Write([]byte("secret"))
	}))
	defer srv.Close()

	c := newSafeClient(blockedIP, nil)
	resp, err := c.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected an error, the request succeeded")
	}
	if !strings.Contains(err.Error(), "refusing to connect to non-public address") {
		t.Errorf("unexpected error: %v", err)
	}

	// The point of the test: the server never saw the request. The guard blocks
	// before connecting, not after reading the response.
	if reached.Load() {
		t.Error("the server was reached: the guard acts too late")
	}
}

func TestSafeClient_AllowsPublicAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// Same guard, but with loopback treated as public: checks that the custom
	// dialer can also connect, not only refuse.
	c := newSafeClient(func(ip net.IP) bool { return !ip.IsLoopback() && blockedIP(ip) }, nil)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// policy.network: the host allowlist
// ---------------------------------------------------------------------------

func TestFetch_HostNotAllowed(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
	}))
	defer srv.Close()

	ft := newTestTool(WithHostAllowed(func(host string) bool { return host == "api.github.com" }))

	_, err := ft.Execute(context.Background(), map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "host not allowed") {
		t.Errorf("unexpected error: %v", err)
	}
	if reached.Load() {
		t.Error("the server was reached even though its host is not in the allowlist")
	}
}

func TestFetch_HostAllowed(t *testing.T) {
	srv := serve(t, "text/plain", "content")
	ft := newTestTool(WithHostAllowed(func(host string) bool { return host == "127.0.0.1" }))

	got, err := ft.Execute(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "content" {
		t.Errorf("output = %q", got)
	}
}

// Without WithHostAllowed the check is off: no allowlist in the manifest means
// "any host", not "no host".
func TestFetch_NoAllowlistMeansAnyHost(t *testing.T) {
	srv := serve(t, "text/plain", "unrestricted")
	got, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "unrestricted" {
		t.Errorf("output = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Redirects: the allowlist must hold past the first hop, otherwise a single 302
// from an allowed host is enough to leave the fence.
// ---------------------------------------------------------------------------

func TestFetch_RedirectToDisallowedHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.invalid/leak", http.StatusFound)
	}))
	defer srv.Close()

	ft := newTestTool(WithHostAllowed(func(host string) bool { return host == "127.0.0.1" }))

	_, err := ft.Execute(context.Background(), map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	// The .invalid TLD never resolves: a DNS error here would mean the redirect
	// was followed and the policy filtered nothing.
	if !strings.Contains(err.Error(), "not allowed by policy.network") {
		t.Errorf("error = %v, want the policy refusal", err)
	}
}

func TestFetch_RedirectToNonHTTPScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://evil.invalid/x", http.StatusFound)
	}))
	defer srv.Close()

	_, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing redirect to scheme") {
		t.Errorf("error = %v, want the scheme refusal", err)
	}
}

func TestFetch_TooManyRedirects(t *testing.T) {
	var hops atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/hop%d", hops.Load()), http.StatusFound)
	}))
	defer srv.Close()

	_, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("error = %v, want the redirect limit", err)
	}
	if got := hops.Load(); got > maxRedirects+1 {
		t.Errorf("the client made %d hops, the limit is %d", got, maxRedirects)
	}
}

func TestFetch_FollowsAllowedRedirect(t *testing.T) {
	dst := serve(t, "text/plain", "destination")
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dst.URL, http.StatusFound)
	}))
	defer src.Close()

	ft := newTestTool(WithHostAllowed(func(host string) bool { return host == "127.0.0.1" }))

	got, err := ft.Execute(context.Background(), map[string]any{"url": src.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "destination" {
		t.Errorf("output = %q, want the destination body", got)
	}
}

// ---------------------------------------------------------------------------
// Response body
// ---------------------------------------------------------------------------

func TestFetch_PlainTextIsNotStripped(t *testing.T) {
	// Markup is removed only when the server says text/html: on text/plain the
	// tags are content, not structure.
	srv := serve(t, "text/plain", "<b>bold</b>")

	got, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "<b>bold</b>" {
		t.Errorf("output = %q, plain text must be left alone", got)
	}
}

func TestFetch_HTMLIsConverted(t *testing.T) {
	srv := serve(t, "text/html; charset=utf-8",
		`<html><head><title>Title</title><script>x=1</script></head><body><p>First</p><p>Second</p></body></html>`)

	got, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"# Title", "First", "Second"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\noutput:\n%s", want, got)
		}
	}
	if strings.Contains(got, "x=1") {
		t.Errorf("the script leaked into the output:\n%s", got)
	}
}

func TestFetch_NonSuccessStatusIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("page missing"))
	}))
	defer srv.Close()

	// An error status is not a Go error: the model must be able to read it and
	// decide, exactly as with any other tool result.
	got, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasPrefix(got, "HTTP 404 Not Found") {
		t.Errorf("output = %q, want the status first", got)
	}
	if !strings.Contains(got, "page missing") {
		t.Errorf("the body was lost: %q", got)
	}
}

func TestFetch_EmptyBodyIsAnError(t *testing.T) {
	srv := serve(t, "text/plain", "   \n\t  ")

	_, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no content") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFetch_TruncatesLongBody(t *testing.T) {
	body := strings.Repeat("a", maxResultChars+10000)
	srv := serve(t, "text/plain", body)

	got, err := newTestTool().Execute(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasPrefix(got, strings.Repeat("a", maxResultChars)) {
		t.Error("the first maxResultChars characters were not returned")
	}
	// The truncation note is for the model: without it, it believes it read the
	// whole page.
	want := fmt.Sprintf("(truncated: the page is %d characters, showing the first %d)", len(body), maxResultChars)
	if !strings.Contains(got, want) {
		t.Errorf("missing the truncation note %q", want)
	}
}

func TestFetch_ContextCancelled(t *testing.T) {
	srv := serve(t, "text/plain", "never read")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newTestTool().Execute(ctx, map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected an error with a cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// htmlToText
// ---------------------------------------------------------------------------

func TestHTMLToText(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		contains []string
		absent   []string
	}{
		{
			name: "the title becomes a heading",
			// The title lives inside <head>, which is dropped later: it is
			// extracted first precisely for that reason.
			in:       `<html><head><title>Incident 42</title></head><body>body text</body></html>`,
			contains: []string{"# Incident 42", "body text"},
		},
		{
			name:     "script, style, noscript and svg are dropped",
			in:       `<body>visible<script>var secret=1</script><style>.a{color:red}</style><noscript>js off</noscript><svg><path d="M0"/></svg></body>`,
			contains: []string{"visible"},
			absent:   []string{"secret", "color:red", "js off", "M0"},
		},
		{
			name:     "comments are dropped",
			in:       `<p>text</p><!-- TODO: remove the sk-xxx key -->`,
			contains: []string{"text"},
			absent:   []string{"TODO", "sk-xxx"},
		},
		{
			name: "block tags become line breaks",
			// Opening and closing tags both count, so two paragraphs stay two
			// newlines apart: that blank line is what separates the blocks.
			in:       `<p>one</p><p>two</p><li>three</li>`,
			contains: []string{"one\n\ntwo\n\nthree"},
		},
		{
			name:     "entities are decoded",
			in:       `<p>Tom &amp; Jerry &lt;3</p>`,
			contains: []string{"Tom & Jerry <3"},
			absent:   []string{"&amp;", "&lt;"},
		},
		{
			name: "nbsp becomes an ordinary space",
			in:   `<p>a&nbsp;b</p>`,
			// &nbsp; decodes to U+00A0 and is then collapsed by the whitespace
			// regexp: otherwise the model would receive spaces that are not
			// spaces.
			contains: []string{"a b"},
		},
		{
			name:     "runs of blank lines are collapsed",
			in:       "<div>a</div>\n\n\n\n\n<div>b</div>",
			contains: []string{"a\n\nb"},
		},
		{
			name:     "no title means no heading is added",
			in:       `<body>body only</body>`,
			contains: []string{"body only"},
			absent:   []string{"#"},
		},
		{
			name:     "plain text survives untouched",
			in:       `no tags here`,
			contains: []string{"no tags here"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := htmlToText(tc.in)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q\noutput:\n%s", want, got)
				}
			}
			for _, no := range tc.absent {
				if strings.Contains(got, no) {
					t.Errorf("present but not expected %q\noutput:\n%s", no, got)
				}
			}
		})
	}
}

func TestStripTags(t *testing.T) {
	if got := stripTags(`<b>a</b><i k="v">b</i>`); got != "ab" {
		t.Errorf("stripTags = %q, want %q", got, "ab")
	}
}

// ---------------------------------------------------------------------------
// truncateUTF8
// ---------------------------------------------------------------------------

func TestTruncateUTF8(t *testing.T) {
	t.Run("string shorter than the limit", func(t *testing.T) {
		if got := truncateUTF8("abc", 10); got != "abc" {
			t.Errorf("= %q, want %q", got, "abc")
		}
	})

	t.Run("exact cut", func(t *testing.T) {
		if got := truncateUTF8("abcdef", 3); got != "abc" {
			t.Errorf("= %q, want %q", got, "abc")
		}
	})

	t.Run("cut in the middle of a rune", func(t *testing.T) {
		// An accented "e" takes two bytes: cutting at 5 would land inside the
		// third one, and the model would receive invalid UTF-8.
		s := strings.Repeat("é", 10)
		got := truncateUTF8(s, 5)
		if !utf8.ValidString(got) {
			t.Errorf("output is not valid UTF-8: %q", got)
		}
		if len(got) != 4 {
			t.Errorf("len = %d, want 4 (two complete runes)", len(got))
		}
	})
}
