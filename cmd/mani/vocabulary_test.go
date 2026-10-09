package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vocabularyManifest = `
identity:
  name: classifier
  provider: ollama
  model: test-model
  prompt: "classify"
output:
  schema:
    type: object
    properties:
      person: { type: string, enum: !include ./people.txt }
    required: [person]
`

// writeVocabularyProject writes a manifest and its vocabulary file together.
func writeVocabularyProject(t *testing.T, people string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"agent.yaml": vocabularyManifest, "people.txt": people} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "agent.yaml")
}

// A large vocabulary is legal: validate says so and warns, and the warning
// goes to stderr so `mani validate | …` keeps working.
func TestValidate_WarnsOnALargeEnum(t *testing.T) {
	people := make([]string, 250)
	for i := range people {
		people[i] = fmt.Sprintf("person %d", i)
	}
	manifest := writeVocabularyProject(t, strings.Join(people, "\n")+"\n")

	res := runMani(t, t.TempDir(), "validate", "--config", manifest)
	if res.code != 0 {
		t.Fatalf("exit %d, want 0: a big enum is legal\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "ok") || !strings.Contains(res.stdout, "structured") {
		t.Errorf("stdout = %q", res.stdout)
	}
	if !strings.Contains(res.stderr, "warning:") || !strings.Contains(res.stderr, "output.schema.person.enum") {
		t.Errorf("stderr = %q, want a warning naming the field", res.stderr)
	}
	if strings.Contains(res.stdout, "warning") {
		t.Errorf("the warning is on stdout, which is the output of the command: %q", res.stdout)
	}
}

func TestValidate_QuietOnASmallEnum(t *testing.T) {
	manifest := writeVocabularyProject(t, "Isabella\nFrancesco\n")

	res := runMani(t, t.TempDir(), "validate", "--config", manifest)
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	if strings.Contains(res.stderr, "warning") {
		t.Errorf("stderr = %q, want no warning", res.stderr)
	}
}

// A vocabulary that cannot be read is a usage error, before anything runs.
func TestValidate_BrokenVocabularyIsAUsageError(t *testing.T) {
	manifest := writeVocabularyProject(t, "Isabella\nIsabella\n")

	res := runMani(t, t.TempDir(), "validate", "--config", manifest)
	if res.code != exitUsage {
		t.Errorf("exit %d, want %d", res.code, exitUsage)
	}
	if !strings.Contains(res.stderr, "duplicate value") || !strings.Contains(res.stderr, "output.schema.person.enum") {
		t.Errorf("stderr = %q, want the reason and the field", res.stderr)
	}
}

// The same file must be refused by `mani run`, not only by validate: a manifest
// that validates and runs are the same manifest.
func TestRun_BrokenVocabularyFailsBeforeTheModel(t *testing.T) {
	llm := newFakeLLM(t)
	home := cliHome(t, llm.srv.URL)
	manifest := writeVocabularyProject(t, "# nothing but a comment\n")

	res := runMani(t, home, "run", "--config", manifest, "--task", "classify this")
	if res.code == 0 {
		t.Fatalf("exit 0 with an empty vocabulary\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "no values") {
		t.Errorf("stderr = %q, want the reason", res.stderr)
	}
	if llm.calls.Load() != 0 {
		t.Error("the model was called with a broken schema")
	}
}

// An enum in the schema has to reach the provider, or the vocabulary is
// decoration: the tool definition mani sends must carry the values.
func TestRun_EnumReachesTheProvider(t *testing.T) {
	llm := newFakeLLM(t)
	llm.reply = func(string) map[string]any { return map[string]any{"person": "Isabella"} }
	home := cliHome(t, llm.srv.URL)
	manifest := writeVocabularyProject(t, "Isabella\nFrancesco\n")

	res := runMani(t, home, "run", "--config", manifest, "--task", "who wrote it")
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.stderr)
	}
	body := llm.lastBody()
	for _, want := range []string{`"Isabella"`, `"Francesco"`, `"enum"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the request to the provider lacks %s:\n%s", want, body)
		}
	}
}
