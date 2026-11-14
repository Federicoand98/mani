package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Federicoand98/mani/server/wire"
)

func folderFixture(t *testing.T) (*fixture, *Server) {
	t.Helper()
	f := &fixture{t: t, dir: t.TempDir()}
	s := New(Options{Dir: f.dir, Token: "folder-test"})
	t.Cleanup(s.Close)
	f.h = s.Handler()
	f.cookie = f.login("folder-test")
	s.pickFolder = func(context.Context, string) (string, bool, error) {
		t.Error("unexpected native dialog launch")
		return "", false, errors.New("unexpected dialog")
	}
	return f, s
}

func TestNativeFolderAPI_AuthenticationAndOrigin(t *testing.T) {
	f, _ := folderFixture(t)
	for _, tc := range []struct {
		name, origin string
		cookie       bool
		status       int
	}{
		{"unsigned", "", false, 401},
		{"foreign origin", "http://elsewhere.test", true, 403},
		{"null origin", "null", true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://"+host+"/api/folders/pick", strings.NewReader("{}"))
			r.Header.Set("Origin", tc.origin)
			if tc.cookie {
				r.AddCookie(f.cookie)
			}
			if w := f.raw(r); w.Code != tc.status {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestNativeFolderAPI_AbsoluteSelectionOutsideWorkspace(t *testing.T) {
	f, s := folderFixture(t)
	outside := t.TempDir()
	s.pickFolder = func(ctx context.Context, start string) (string, bool, error) {
		if start != outside {
			t.Errorf("start = %q, want %q", start, outside)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > nativeFolderTimeout {
			t.Errorf("unbounded dialog context: %v, %v", deadline, ok)
		}
		return outside, false, nil
	}
	var out NativeFolderResult
	if status := f.do("POST", "/api/folders/pick", `{"path":`+quote(outside)+`}`, &out); status != 200 || out.Cancelled || out.Path != outside {
		t.Fatalf("selection: %d, %+v", status, out)
	}
}

func TestNativeFolderAPI_CancelAndErrors(t *testing.T) {
	f, s := folderFixture(t)
	file := filepath.Join(f.dir, "file.txt")
	if err := os.WriteFile(file, []byte("not a folder"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		path      string
		cancelled bool
		err       error
		status    int
		message   string
	}{
		{"cancel", "", true, nil, 200, ""},
		{"unavailable", "", false, fmt.Errorf("%w: zenity is missing", errFolderUnavailable), 503, "zenity"},
		{"command failed", "", false, errors.New("native command crashed"), 500, "crashed"},
		{"timeout", "", false, context.DeadlineExceeded, 504, "deadline"},
		{"relative selection", ".", false, nil, 500, "absolute"},
		{"missing selection", filepath.Join(f.dir, "missing"), false, nil, 500, "cannot open"},
		{"file selection", file, false, nil, 500, "not a directory"},
		{"empty selection", "", false, nil, 500, "absolute"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.pickFolder = func(_ context.Context, start string) (string, bool, error) {
				if start != f.dir {
					t.Errorf("default start = %q, want %q", start, f.dir)
				}
				return tc.path, tc.cancelled, tc.err
			}
			r := httptest.NewRequest("POST", "http://"+host+"/api/folders/pick", strings.NewReader("{}"))
			r.AddCookie(f.cookie)
			w := f.raw(r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.message) {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
			if tc.cancelled && w.Body.String() != "{\"path\":\"\",\"cancelled\":true}\n" {
				t.Fatalf("cancel response: %s", w.Body.String())
			}
		})
	}
	for _, body := range []string{
		`{"unknown":true}`, `{"path":1}`, `{`,
		`{"path":` + quote(file) + `}`,
		`{"path":` + quote(filepath.Join(f.dir, "missing")) + `}`,
	} {
		var out APIError
		if status := f.do("POST", "/api/folders/pick", body, &out); status != 400 {
			t.Errorf("invalid input %s: %d, %+v", body, status, out)
		}
	}
}

func TestNativeFolderAPI_ConcurrentDialogAndRequestCancellation(t *testing.T) {
	f, s := folderFixture(t)
	started := make(chan struct{})
	s.pickFolder = func(ctx context.Context, _ string) (string, bool, error) {
		close(started)
		<-ctx.Done()
		return "", false, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest("POST", "http://"+host+"/api/folders/pick", strings.NewReader("{}")).WithContext(ctx)
	r.AddCookie(f.cookie)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.raw(r) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("dialog was not launched")
	}
	var out APIError
	if status := f.do("POST", "/api/folders/pick", "{}", &out); status != 409 {
		t.Errorf("concurrent selection: %d, %+v", status, out)
	}
	cancel()
	select {
	case w := <-done:
		if w.Code != 408 || !strings.Contains(w.Body.String(), "context canceled") {
			t.Fatalf("request cancellation: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dialog did not stop with the request")
	}
	s.pickFolder = func(context.Context, string) (string, bool, error) { return "", true, nil }
	var result NativeFolderResult
	if status := f.do("POST", "/api/folders/pick", "{}", &result); status != 200 || !result.Cancelled {
		t.Fatalf("dialog lock not released: %d, %+v", status, result)
	}
}

func TestNativeFolderAPI_ServerCloseCancelsDialog(t *testing.T) {
	f, s := folderFixture(t)
	s.pickFolder = func(ctx context.Context, _ string) (string, bool, error) {
		s.Close()
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-time.After(5 * time.Second):
			t.Error("server close did not stop dialog")
			return "", false, errors.New("not cancelled")
		}
	}
	var out APIError
	if status := f.do("POST", "/api/folders/pick", "{}", &out); status != 408 {
		t.Errorf("server close: %d, %+v", status, out)
	}
}

func TestNativeFolderCommand_PlatformArguments(t *testing.T) {
	start := `/outside/folder '"; $HOME` + "`"
	for _, tc := range []struct {
		os, tool string
		args     []string
	}{
		{"linux", "zenity", []string{"--file-selection", "--directory", "--title=Choose agent workspace", "--filename=" + start + string(os.PathSeparator)}},
		{"linux", "kdialog", []string{"--getexistingdirectory", start, "--title", "Choose agent workspace"}},
		{"darwin", "osascript", []string{"-e", nativeFolderAppleScript, "--", start}},
		{"windows", "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-STA", "-Command", nativeFolderPowerShell}},
	} {
		t.Run(tc.os+"/"+tc.tool, func(t *testing.T) {
			p := nativeFolderPicker{
				goos:   tc.os,
				getenv: func(string) string { return ":0" },
				lookPath: func(name string) (string, error) {
					if name != tc.tool {
						return "", exec.ErrNotFound
					}
					return filepath.Join("native-bin", name), nil
				},
			}
			cmd, err := p.command(context.Background(), start)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != filepath.Join("native-bin", tc.tool) || !reflect.DeepEqual(cmd.Args[1:], tc.args) {
				t.Fatalf("command = %s %q", cmd.Path, cmd.Args)
			}
			if tc.os == "windows" {
				if !strings.Contains(strings.Join(cmd.Env, "\n"), "MANI_FOLDER_START="+start) || strings.Contains(nativeFolderPowerShell, start) {
					t.Fatal("Windows must pass the path through the environment, not script interpolation")
				}
			}
		})
	}
}

func TestNativeFolderCommand_Unavailable(t *testing.T) {
	for _, tc := range []struct {
		name, goos, display string
		tools               bool
	}{
		{"headless", "linux", "", true},
		{"missing Linux tool", "linux", ":0", false},
		{"missing macOS tool", "darwin", "", false},
		{"missing Windows tool", "windows", "", false},
		{"unsupported", "freebsd", ":0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := nativeFolderPicker{
				goos: tc.goos, getenv: func(string) string { return tc.display },
				lookPath: func(name string) (string, error) {
					if tc.tools {
						return name, nil
					}
					return "", exec.ErrNotFound
				},
			}
			if _, err := p.command(context.Background(), "/"); !errors.Is(err, errFolderUnavailable) {
				t.Fatalf("unavailable error = %v", err)
			}
		})
	}
}

// Only this test process is executed; native desktop commands are never run.
func TestNativeFolderProcessHelper(t *testing.T) {
	switch os.Getenv("MANI_FOLDER_TEST_EXIT") {
	case "1":
		os.Exit(1)
	case "2":
		os.Exit(2)
	case "3":
		os.Exit(3)
	}
}

func TestNativeFolderPicker_ResultsAndCommandErrors(t *testing.T) {
	exitError := func(code string) error {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestNativeFolderProcessHelper$")
		cmd.Env = append(os.Environ(), "MANI_FOLDER_TEST_EXIT="+code)
		err := cmd.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("helper exit: %v", err)
		}
		return err
	}
	cancelExit, crashExit, headlessExit := exitError("1"), exitError("2"), exitError("3")
	for _, tc := range []struct {
		name, goos, output, stderr string
		err                        error
		cancelled, fails           bool
	}{
		{"Linux selection preserves spaces", "linux", "/outside/ spaced \n", "", nil, false, false},
		{"Linux cancel", "linux", "", "", cancelExit, true, false},
		{"Linux empty success", "linux", "", "", nil, false, true},
		{"Linux display failure", "linux", "", "cannot open display", cancelExit, false, true},
		{"Linux crash", "linux", "", "", crashExit, false, true},
		{"Linux crash with output", "linux", "/some/path\n", "", cancelExit, false, true},
		{"macOS cancel", "darwin", "\n", "", nil, true, false},
		{"macOS error not cancel", "darwin", "", "execution error", cancelExit, false, true},
		{"macOS headless", "darwin", "", "execution error: application isn't running (-600)", cancelExit, false, true},
		{"Windows cancel", "windows", "", "", nil, true, false},
		{"Windows Unicode path", "windows", "C:\\caffè\\文書\r\n", "", nil, false, false},
		{"Windows crash", "windows", "", "failed", crashExit, false, true},
		{"Windows headless", "windows", "", "", headlessExit, false, true},
		{"launch error", "linux", "", "", exec.ErrNotFound, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := nativeFolderPicker{
				goos: tc.goos, getenv: func(string) string { return ":0" },
				lookPath: func(name string) (string, error) { return name, nil },
				output: func(cmd *exec.Cmd) ([]byte, error) {
					fmt.Fprint(cmd.Stderr, tc.stderr)
					if cmd.WaitDelay <= 0 {
						t.Error("unbounded command pipe wait")
					}
					return []byte(tc.output), tc.err
				},
			}
			path, cancelled, err := p.pick(context.Background(), "/start")
			if (err != nil) != tc.fails || cancelled != tc.cancelled {
				t.Fatalf("path %q, cancelled %v, err %v", path, cancelled, err)
			}
			if !tc.fails && !tc.cancelled && path != strings.TrimSuffix(strings.TrimSuffix(tc.output, "\n"), "\r") {
				t.Errorf("selected path = %q", path)
			}
			if (strings.Contains(tc.name, "headless") || tc.name == "Linux display failure" || tc.name == "launch error") && !errors.Is(err, errFolderUnavailable) {
				t.Errorf("headless error = %v", err)
			}
		})
	}
}

func TestNativeFolderPicker_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := nativeFolderPicker{
		goos: "linux", getenv: func(string) string { return ":0" },
		lookPath: func(name string) (string, error) { return name, nil },
		output: func(*exec.Cmd) ([]byte, error) {
			cancel()
			return nil, errors.New("signal: killed")
		},
	}
	if _, cancelled, err := p.pick(ctx, "/start"); cancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation is not a dialog cancel: %v, %v", cancelled, err)
	}
}

func TestNativeFolderWorkspace_AgentsAndDraftsRespectAbsoluteWorkspace(t *testing.T) {
	for _, draft := range []bool{false, true} {
		t.Run(fmt.Sprintf("draft=%v", draft), func(t *testing.T) {
			e := newAgentEnv(t)
			outside := t.TempDir()
			write(t, outside, "notes.txt", "outside workspace selected by user\n")
			text := strings.Replace(agentYAML, "capabilities:\n", "capabilities:\n  workspace: "+quote(outside)+"\n", 1)
			var path string
			if draft {
				var d Draft
				if code := e.api("POST", "/api/drafts", `{"text":`+quote(text)+`}`, &d); code != 201 {
					t.Fatalf("create draft: %d", code)
				}
				path = "/api/drafts/" + d.ID + "/ws"
			} else {
				write(t, e.dir, "agent.yaml", text)
				c := e.createWithAgent("agent.yaml")
				path = "/api/conversations/" + c.ID + "/ws"
			}
			ws, err := e.dialPath(path, nil)
			if err != nil {
				t.Fatal(err)
			}
			ws.send(wire.ClientMsg{Type: "input", Input: "please read it"})
			msgs := ws.collect("done", "error")
			found := false
			for _, m := range msgs {
				if m.Type == "tool_result" && strings.Contains(payload(m)["result"].(string), "outside workspace selected by user") {
					found = true
				}
			}
			if !found || msgs[len(msgs)-1].Type != "done" {
				t.Fatalf("absolute workspace not used: %+v", msgs)
			}
		})
	}
}
