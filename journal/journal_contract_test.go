package journal_test

import (
	"testing"

	"github.com/Federicoand98/mani/journal"
	"github.com/Federicoand98/mani/journal/journaltest"
)

func TestContract_InMemory(t *testing.T) {
	journaltest.Run(t, journal.NewInMemory(10))
}

func TestContract_JSONL(t *testing.T) {
	j, err := journal.NewJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journaltest.Run(t, j)
}

// What the runtime actually uses when a journal is configured: memory in front
// of disk. The fan-out must not change what a reader sees.
func TestContract_Multi(t *testing.T) {
	disk, err := journal.NewJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journaltest.Run(t, journal.NewMulti(journal.NewInMemory(10), disk))
}
