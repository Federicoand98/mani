package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// record is what travels on every edge of a flow: an id, a payload and whatever esle the producer attached.
type record map[string]any

func (r record) id() string {
	s, _ := r["id"].(string)
	return s
}

// carry keeps the producer's own fields and drops the ones every hop rewrites.
func (r record) carry() record {
	out := make(record, len(r)+3)
	for k, v := range r {
		switch k {
		case "id", "task", "result", "run":
		default:
			out[k] = v
		}
	}

	return out
}

// taskText turns a payload into what an agent receives.
// A string goes as it it, anything else goes as JSON.
func taskText(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", errors.New("no task")
	case string:
		if strings.TrimSpace(t) == "" {
			return "", errors.New("empty task")
		}
		return t, nil
	default:
		b, err := json.Marshal(t)
		return string(b), err
	}
}

// readRecordsFrom reads --in: a file or "-" for stdin.
func readRecordsFrom(path string) ([]record, error) {
	var recs []record
	var err error
	if path == "-" {
		recs, err = parseRecords(os.Stdin, "stdin")
	} else {
		var f *os.File
		if f, err = os.Open(path); err != nil {
			return nil, err
		}
		defer f.Close()
		recs, err = parseRecords(f, path)
	}

	if err == nil && len(recs) == 0 {
		err = fmt.Errorf("%s: no records", path)
	}

	return recs, err
}

// parseRecords reads JSONL, one object per line, each with an id that can be a filename and is not repeated.
// The whole input is checked before anything runs: a malformed line must fail now, not two thousand runs from now.
func parseRecords(r io.Reader, source string) ([]record, error) {
	var recs []record
	seen := map[string]int{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	line := 0

	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}

		var rec record
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			return nil, fmt.Errorf("%s line %d: not a JSON object: %w", source, line, err)
		}
		id := rec.id()
		if id == "" {
			return nil, fmt.Errorf(`%s line %d: "id" is required and must be a string`, source, line)
		}
		if id != filepath.Base(id) || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
			return nil, fmt.Errorf("%s line %d: id %q cannot be a file name", source, line, id)
		}
		if prev, dup := seen[id]; dup {
			return nil, fmt.Errorf("%s line %d: id %q already used on line %d", source, line, id, prev)
		}
		seen[id] = line
		recs = append(recs, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return recs, nil
}

// stepOutput is what a later step needs from an earlier one.
type stepOutput struct {
	records []record
	field   string               // where the payload is: "task" or "result"
	stamp   map[string]time.Time // when each record was written; missing = zero
	newest  time.Time            // the latest of them
}

// stepStore is one step's directory: <id>.json per record, errors.jsonl for
// the failed attempts, and .done once a whole-step run has finished.
type stepStore struct {
	dir string
	mu  sync.Mutex // guards errors.jsonl
}

func (s *stepStore) path(id string) string { return filepath.Join(s.dir, id+".json") }

func (s *stepStore) put(rec record) error { return writeJSONAtomic(s.path(rec.id()), rec) }

// failed appends, never truncates: the history of attempts is data too.
func (s *stepStore) failed(id string, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.dir, "errors.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(map[string]any{
		"id":    id,
		"error": cause.Error(),
		"at":    time.Now().UTC().Format(time.RFC3339),
	})
}

// freshSince is make's rule for one record: it exists and is not older than
// the record it was made from.
func (s *stepStore) freshSince(id string, since time.Time) bool {
	fi, err := os.Stat(s.path(id))
	return err == nil && !fi.ModTime().Before(since)
}

// done is make's rule for a whole-step run: it finished after the newest
// record it read.
func (s *stepStore) done(since time.Time) bool {
	fi, err := os.Stat(filepath.Join(s.dir, ".done"))
	return err == nil && !fi.ModTime().Before(since)
}

// replace makes the directory hold exactly recs. Identical files are left
// alone — their time does not move, so the steps after them stay done — the
// others are written, those no longer produced are removed. .done goes last.
func (s *stepStore) replace(recs []record) error {
	keep := map[string]bool{}
	for _, rec := range recs {
		keep[rec.id()+".json"] = true
		if err := s.put(rec); err != nil {
			return err
		}
	}
	old, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return err
	}
	for _, p := range old {
		if !keep[filepath.Base(p)] {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	marker := filepath.Join(s.dir, ".done")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		return err
	}
	now := time.Now()
	return os.Chtimes(marker, now, now) // rewriting an empty file does not move its time everywhere
}

// output reads the step's records back: with ids, those that exist, in that
// order; without, every record in the directory, sorted by id.
func (s *stepStore) output(ids []string, field string) (stepOutput, error) {
	if ids == nil {
		paths, err := filepath.Glob(filepath.Join(s.dir, "*.json")) // sorted
		if err != nil {
			return stepOutput{}, err
		}
		for _, p := range paths {
			ids = append(ids, strings.TrimSuffix(filepath.Base(p), ".json"))
		}
	}

	out := stepOutput{field: field, stamp: map[string]time.Time{}}
	for _, id := range ids {
		fi, err := os.Stat(s.path(id))
		if errors.Is(err, os.ErrNotExist) {
			continue // failed, or left for later by --limit
		}
		if err != nil {
			return stepOutput{}, err
		}
		b, err := os.ReadFile(s.path(id))
		if err != nil {
			return stepOutput{}, err
		}
		var rec record
		if err := json.Unmarshal(b, &rec); err != nil {
			return stepOutput{}, fmt.Errorf("%s: %w", s.path(id), err)
		}
		out.records = append(out.records, rec)
		out.stamp[id] = fi.ModTime()
		if fi.ModTime().After(out.newest) {
			out.newest = fi.ModTime()
		}
	}
	return out, nil
}

// writeJSONAtomic writes through a temporary file, so an interrupted run never
// leaves half a record that the next run would count as done. An identical
// file is not rewritten: its time is what tells the steps after it they are
// done.
func writeJSONAtomic(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// recordStream writes records to stdout as JSONL. With it stdout is a format:
// one writer, one line per record, and nothing else ever goes there.
type recordStream struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func newRecordStream(w io.Writer) *recordStream { return &recordStream{enc: json.NewEncoder(w)} }

func (s *recordStream) write(rec record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(rec)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
