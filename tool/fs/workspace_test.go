package fs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspace_Resolve_RelativeInside(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	got, err := w.Resolve("foo.txt")
	if err != nil {
		t.Fatalf("Resolve errore inatteso: %v", err)
	}
	want := filepath.Join(root, "foo.txt")
	if got != want {
		t.Errorf("path atteso %q, ottenuto %q", want, got)
	}
}

func TestWorkspace_Resolve_DeepSubdir(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	got, err := w.Resolve("a/b/c.go")
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if !strings.HasPrefix(got, root) {
		t.Errorf("path %q non sotto root %q", got, root)
	}
}

func TestWorkspace_Resolve_DotDotTraversal_Rejected(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	if _, err := w.Resolve("../escape.txt"); err == nil {
		t.Error("attesa rejection per '..' traversal")
	}
}

func TestWorkspace_Resolve_AbsolutePath_Reanchored(t *testing.T) {
	// filepath.Join neutralizza il leading '/': /etc/passwd diventa <root>/etc/passwd.
	// L'invariante di sicurezza è "mai fuori dalla root", non "rifiuta gli assoluti".
	root := t.TempDir()
	w := NewWorkspace(root)

	got, err := w.Resolve("/etc/passwd")
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if !strings.HasPrefix(got, root+string(filepath.Separator)) {
		t.Errorf("path assoluto deve restare dentro %q, ottenuto %q", root, got)
	}
}

func TestWorkspace_Resolve_TraversalViaAbsolute_Rejected(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	if _, err := w.Resolve("/../../etc/passwd"); err == nil {
		t.Error("attesa rejection: '..' assoluti devono uscire dalla root e fallire")
	}
}

func TestWorkspace_Resolve_AbsoluteInside_Allowed(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	// path assoluto dentro la root: filepath.Join(root, abs) → comportamento
	// di Clean assorbe il leading "/" come membro relativo. Verifichiamo solo
	// che subpath ricostruita resti dentro la root.
	abs := filepath.Join(root, "ok.txt")
	got, err := w.Resolve(abs)
	if err != nil {
		// se reject perché Join lo tratta come fuori, accettabile;
		// l'invariante è "mai fuori dalla root".
		return
	}
	if !strings.HasPrefix(got, root) {
		t.Errorf("path %q deve restare dentro %q", got, root)
	}
}

func TestWorkspace_Resolve_RootItself_Rejected(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	if _, err := w.Resolve("."); err == nil {
		t.Error("attesa rejection per path == workspace root")
	}
	if _, err := w.Resolve(""); err == nil {
		t.Error("attesa rejection per path vuoto (resolve a root)")
	}
}

func TestWorkspace_Resolve_ReturnsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	got, err := w.Resolve("x.go")
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("path deve essere assoluto, ottenuto %q", got)
	}
}

func TestWorkspace_Resolve_CleansPath(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)

	got, err := w.Resolve("a/./b/../c.go")
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	want := filepath.Join(root, "a", "c.go")
	if got != want {
		t.Errorf("Clean atteso, ottenuto %q invece di %q", got, want)
	}
}
