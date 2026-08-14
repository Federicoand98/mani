package fs

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "tar": true,
	".venv": true, "venv": true, "__pycache__": true, ".next": true, ".idea": true, ".vscode": true,
	".mani": true, ".tox": true, ".pytest_cache": true,
}

const (
	maxGlobResults = 500
	maxGrepMatches = 100
	maxLineChars   = 200
	maxFileBytes   = 2 << 20
	binarySniff    = 8 << 10
)

var errStopWalk = errors.New("fs: walk stopped")

func walkFiles(ctx context.Context, root, sub string, fn func(rel, abs string) (bool, error)) error {
	start := root
	if sub != "" {
		start = filepath.Join(root, sub)
	}

	err := filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			if path != start && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}

		rel = filepath.ToSlash(rel)

		cont, ferr := fn(rel, path)
		if ferr != nil {
			return ferr
		}
		if !cont {
			return errStopWalk
		}

		return nil
	})

	if errors.Is(err, errStopWalk) {
		return nil
	}

	return err
}

// globToRegexp converts a glob pattern to a regexp.
// *  everything execpt "/"
// ** everything including "/"
// ?  a character except "/"
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, err
	}
	return re, nil
}

// isBinaryFile: a NULL byte in the first 8KB (binarysniff)
func isBinaryFile(data []byte) bool {
	head := data
	if len(head) > binarySniff {
		head = head[:binarySniff]
	}
	return bytes.IndexByte(head, 0) >= 0
}

func truncateLine(s string) string {
	s = strings.TrimRight(s, "\r")
	if len(s) <= maxLineChars {
		return s
	}
	cut := maxLineChars
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + " ...(line truncated)"
}

func readTextFile(abs string) (string, bool) {
	info, err := os.Stat(abs)
	if err != nil || info.Size() > maxFileBytes {
		return "", false
	}
	data, err := os.ReadFile(abs)
	if err != nil || isBinaryFile(data) {
		return "", false
	}

	return string(data), true
}
