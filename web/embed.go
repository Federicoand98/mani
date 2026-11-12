// Package web holds the browser console, built by SvelteKit into build/ and
// embedded in the mani binary. Everything else in this directory is the
// frontend project; this is its only Go file.
package web

import (
	"embed"
	"io/fs"
)

// build/ always exists: a .gitkeep is committed so that `go build ./...` works on
// a clean checkout. Without `make web` it holds no index.html, and the server
// shows a page that says so.
//
//go:embed all:build
var build embed.FS

// Assets returns the built console rooted at build/.
func Assets() fs.FS {
	sub, _ := fs.Sub(build, "build") // fails only on an invalid path; "build" is a literal
	return sub
}
