package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/Federicoand98/mani/server/workspace"
	"github.com/Federicoand98/mani/web"
)

func runWeb(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	dir := fs.String("dir", ".", "the working directory the console manages")
	addr := fs.String("addr", "127.0.0.1:7878", "listen address, loopback only")
	noOpen := fs.Bool("no-open", false, "print the link without opening the browser")
	_ = fs.Bool("verbose", false, "logs to stderr")
	_ = fs.Bool("debug", false, "alias for --verbose")
	_ = fs.Parse(args)

	if fs.NArg() > 0 {
		return usagef("unexpected argument %q (the folder goes in --dir)", fs.Arg(0))
	}
	if !workspace.IsLoopbackHost(*addr) {
		return usagef("--addr %s: mani web listens only on this machine (127.0.0.1, localhost or ::1)", *addr)
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		return usagef("--dir %s: %v", *dir, err)
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return usagef("--dir %s: not a directory", *dir)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w; is another mani web running? Use --addr to pick another port", *addr, err)
	}
	base := "http://" + ln.Addr().String()
	link := func(token string) string { return base + "/?t=" + token }

	token := workspace.RandomToken()
	srv := workspace.New(workspace.Options{
		Dir:     abs,
		Version: webVersion(),
		Assets:  web.Assets(),
		Token:   token,
		OnLogin: func(next string) {
			fmt.Fprintf(os.Stderr, "mani web: browser signed in. Link for another browser:\n  %s\n", link(next))
		},
	})

	fmt.Fprintf(os.Stderr, "mani web: %s\n  %s\n", abs, link(token))
	if !*noOpen {
		if err := openBrowser(link(token)); err != nil {
			fmt.Fprintf(os.Stderr, "mani web: could not open a browser (%v); open the link above\n", err)
		}
	}

	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// webVersion is the bare version, without the platform suffix of --version.
func webVersion() string {
	if version != "" {
		return version
	}
	bi, _ := debug.ReadBuildInfo()
	return versionFromBuildInfo(bi)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
