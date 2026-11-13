package workspace

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// handleStatic serves the single-page app. A missing file under _app/ is a
// missing asset and answers 404; any other path is a client route, so it gets
// index.html (routes may end in .yaml: /agents/reviewer.yaml).
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.auth.login(w, r) {
		return
	}
	if !s.built {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(notBuiltPage))
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if fi, err := fs.Stat(s.opt.Assets, name); err != nil || fi.IsDir() {
		if strings.HasPrefix(name, "_app/") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
	}

	if strings.HasPrefix(name, "_app/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	f, err := s.opt.Assets.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	rs, ok := f.(io.ReadSeeker)
	if err != nil || !ok {
		http.Error(w, "cannot read "+name, http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, fi.ModTime(), rs)
}

const notBuiltPage = `<!doctype html><meta charset="utf-8"><title>mani web</title>
<body style="font:16px/1.5 system-ui;max-width:36rem;margin:15vh auto;padding:0 1rem">
<h1 style="font-size:1.4rem">The console is not built into this binary</h1>
<p>This mani was compiled without the web console. From the repository, run
<code>make web</code> and build again, or install a release build.</p>
<p>The API under <code>/api</code> works anyway.</p></body>`
