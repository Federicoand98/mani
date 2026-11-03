package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Federicoand98/mani/journal"
	"github.com/Federicoand98/mani/journal/sqlite"
	"github.com/Federicoand98/mani/manifest"
)

func TestJournalBackendSelection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	spec := manifest.Default()
	spec.Observability.Journal = manifest.JournalSpec{
		Enabled: true, Backend: "sqlite", Path: dbPath, Retention: 2,
	}

	reader, ok := OpenJournalReader(spec)
	if !ok {
		t.Fatal("OpenJournalReader should open the SQLite backend")
	}
	if _, ok := reader.(*sqlite.Journal); !ok {
		t.Fatalf("OpenJournalReader returned %T, want *sqlite.Journal", reader)
	}
	defer reader.Close()

	built, err := buildJournal(spec.Observability.Journal)
	if err != nil {
		t.Fatalf("buildJournal: %v", err)
	}
	if _, ok := built.(*journal.Multi); !ok {
		t.Fatalf("buildJournal returned %T, want *journal.Multi", built)
	}
	// What reaches the runtime's journal must land in the SQLite file: write
	// through it, then read the file back through a second handle.
	if err := built.Start(journal.RunRecord{ID: "r1", Source: "test", StartedAt: time.Now()}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := built.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	disk, err := sqlite.New(dbPath, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	if _, err := disk.Get("r1"); err != nil {
		t.Fatalf("the run did not reach the SQLite file: %v", err)
	}
}

func TestRuntimeCloseClosesSQLiteJournal(t *testing.T) {
	j, err := sqlite.New(filepath.Join(t.TempDir(), "runs.db"), 1)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	rt := &Runtime{journal: j}
	rt.Close()

	if _, err := j.List(journal.ListFilter{}); err == nil {
		t.Fatal("Runtime.Close should close the SQLite journal")
	}
}
