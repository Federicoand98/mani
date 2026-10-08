package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vocabularyManifest declares a classifier whose label comes from a file.
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
      person: { type: string, enum: !include %s }
    required: [person]
`

// writeVocabulary writes a manifest and its vocabulary file side by side, the
// way a real project keeps them, and returns the manifest path.
func writeVocabulary(t *testing.T, manifest, vocabName, vocab string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, vocabName), []byte(vocab), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func enumOf(t *testing.T, spec RuntimeSpec, prop string) []string {
	t.Helper()
	p, ok := spec.Output.Schema.Properties[prop]
	if !ok {
		t.Fatalf("no property %q in the schema", prop)
	}
	if p.Enum == nil {
		t.Fatalf("property %q has no enum", prop)
	}
	return p.Enum.Values
}

// A text file is one value per line, with blank lines and # comments allowed:
// it is a list a person maintains by hand.
func TestLoadManifest_EnumIncludeFromTextFile(t *testing.T) {
	path := writeVocabulary(t,
		fmt.Sprintf(vocabularyManifest, "./people.txt"),
		"people.txt",
		"# the people of the busta\n\nIsabella d'Este\nFrancesco Gonzaga\n  Elisabetta Gonzaga  \n")

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	got := enumOf(t, spec, "person")
	want := []string{"Isabella d'Este", "Francesco Gonzaga", "Elisabetta Gonzaga"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("enum = %q, want %q (comments and blank lines dropped, values trimmed)", got, want)
	}
}

// The extension decides how the file is read, so .json and .yaml are lists.
func TestLoadManifest_EnumIncludeFromListFiles(t *testing.T) {
	cases := map[string]string{
		"people.json": `["Isabella", "Francesco"]`,
		"people.yaml": "- Isabella\n- Francesco\n",
		"people.yml":  "[Isabella, Francesco]",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeVocabulary(t, fmt.Sprintf(vocabularyManifest, "./"+name), name, body)

			spec, err := LoadManifest(path)
			if err != nil {
				t.Fatalf("LoadManifest: %v", err)
			}
			if got := enumOf(t, spec, "person"); strings.Join(got, "|") != "Isabella|Francesco" {
				t.Errorf("enum = %q", got)
			}
		})
	}
}

// An enum written inline must keep working: the include is an addition, not a
// replacement.
func TestLoadManifest_InlineEnumStillWorks(t *testing.T) {
	path := writeManifest(t, `
identity:
  name: classifier
  provider: ollama
  model: test-model
  prompt: "classify"
output:
  schema:
    type: object
    properties:
      sentiment: { type: string, enum: [positive, negative] }
`)

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got := enumOf(t, spec, "sentiment"); strings.Join(got, "|") != "positive|negative" {
		t.Errorf("enum = %q", got)
	}
}

// The schema is a tree, and a vocabulary is just as useful on the items of an
// array or on a field of a nested object. Map values are copies in Go, so this
// is exactly where a resolution gets silently lost.
func TestLoadManifest_EnumIncludeInsideArraysAndObjects(t *testing.T) {
	path := writeVocabulary(t, `
identity:
  name: classifier
  provider: ollama
  model: test-model
  prompt: "classify"
output:
  schema:
    type: object
    properties:
      places:
        type: array
        items: { type: string, enum: !include ./places.txt }
      letter:
        type: object
        properties:
          sender: { type: string, enum: !include ./places.txt }
`, "places.txt", "Mantova\nFerrara\n")

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	places := spec.Output.Schema.Properties["places"]
	if places.Items == nil || places.Items.Enum == nil || len(places.Items.Enum.Values) != 2 {
		t.Errorf("items enum = %+v, want the two places", places.Items)
	}
	sender := spec.Output.Schema.Properties["letter"].Properties["sender"]
	if sender.Enum == nil || len(sender.Enum.Values) != 2 {
		t.Errorf("nested enum = %+v, want the two places", sender.Enum)
	}
}

// A subprocess tool declares its own schema: the include has to work there too,
// not only on output.schema.
func TestLoadManifest_EnumIncludeInAToolSchema(t *testing.T) {
	path := writeVocabulary(t, `
identity:
  name: classifier
  provider: ollama
  model: test-model
  prompt: "classify"
capabilities:
  tools:
    - name: lookup
      description: "looks a person up"
      command: ./lookup.py
      risk: none
      schema:
        type: object
        properties:
          person: { type: string, enum: !include ./people.txt }
`, "people.txt", "Isabella\nFrancesco\n")

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	prop := spec.Capabilities.Tools[0].Schema.Properties["person"]
	if prop.Enum == nil || len(prop.Enum.Values) != 2 {
		t.Errorf("tool enum = %+v, want the two people", prop.Enum)
	}
}

// Every way the file can be wrong. A vocabulary that loads half-broken would
// constrain the output to the wrong set, in silence.
func TestLoadManifest_EnumIncludeErrors(t *testing.T) {
	cases := []struct {
		name, file, body, want string
	}{
		{"empty file", "people.txt", "\n\n# only a comment\n", "no values"},
		{"duplicate value", "people.txt", "Isabella\nIsabella\n", `duplicate value "Isabella"`},
		{"json that is not a list", "people.json", `{"a": 1}`, "expected a list of strings"},
		{"json with a nested list", "people.json", `["Isabella", ["Francesco"]]`, "expected a list of strings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeVocabulary(t, fmt.Sprintf(vocabularyManifest, "./"+tc.file), tc.file, tc.body)

			_, err := LoadManifest(path)
			if err == nil {
				t.Fatalf("LoadManifest = nil, want an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to contain %q", err, tc.want)
			}
			// The message must say which field, or a manifest with three
			// vocabularies turns into a hunt.
			if !strings.Contains(err.Error(), "output.schema.person.enum") {
				t.Errorf("err = %q, want it to name the field", err)
			}
		})
	}
}

// A number in a list file becomes its text: an enum is a set of strings, and
// "1495" as a value is more likely a year someone wrote than a mistake worth
// refusing.
func TestLoadManifest_EnumIncludeCoercesScalarsToText(t *testing.T) {
	path := writeVocabulary(t, fmt.Sprintf(vocabularyManifest, "./years.json"), "years.json", `["1495", 1496]`)

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got := enumOf(t, spec, "person"); strings.Join(got, "|") != "1495|1496" {
		t.Errorf("enum = %q", got)
	}
}

func TestLoadManifest_EnumIncludeMissingFile(t *testing.T) {
	path := writeManifest(t, fmt.Sprintf(vocabularyManifest, "./nowhere.txt"))

	_, err := LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "nowhere.txt") {
		t.Errorf("err = %v, want it to name the missing file", err)
	}
}

// Same guard as identity.prompt: the include is relative to the manifest, and
// an absolute path is refused.
func TestLoadManifest_EnumIncludeRefusesAbsolutePaths(t *testing.T) {
	path := writeManifest(t, fmt.Sprintf(vocabularyManifest, "/etc/passwd"))

	_, err := LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("err = %v, want absolute paths refused", err)
	}
}

// The probe decode runs on the original bytes for exactly this reason: an
// include in the file must not cost the line numbers of other errors.
func TestLoadManifest_EnumIncludeKeepsLineNumbers(t *testing.T) {
	path := writeVocabulary(t, `identity:
  name: classifier
  provider: ollama
  model: test-model
  prompt: "classify"
output:
  schema:
    type: object
    properties:
      person: { type: string, enum: !include ./people.txt }
surprise: true
`, "people.txt", "Isabella\n")

	_, err := LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "surprise") || !strings.Contains(err.Error(), "line 11") {
		t.Errorf("err = %v, want the unknown field with its line", err)
	}
}

func TestWarnings_LargeEnum(t *testing.T) {
	big := make([]string, enumWarnThreshold+1)
	for i := range big {
		big[i] = fmt.Sprintf("p%d", i)
	}
	path := writeVocabulary(t,
		fmt.Sprintf(vocabularyManifest, "./people.txt"),
		"people.txt",
		strings.Join(big, "\n")+"\n")

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	warnings := spec.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("warnings = %q, want exactly one", warnings)
	}
	if !strings.Contains(warnings[0], "output.schema.person.enum") ||
		!strings.Contains(warnings[0], fmt.Sprint(enumWarnThreshold+1)) {
		t.Errorf("warning = %q, want the field and how many values", warnings[0])
	}
}

// At the threshold there is no warning: a warning on a legal manifest that
// nobody intends to change is noise, and noise gets ignored.
func TestWarnings_AtTheThresholdIsQuiet(t *testing.T) {
	big := make([]string, enumWarnThreshold)
	for i := range big {
		big[i] = fmt.Sprintf("p%d", i)
	}
	path := writeVocabulary(t,
		fmt.Sprintf(vocabularyManifest, "./people.txt"),
		"people.txt",
		strings.Join(big, "\n")+"\n")

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if w := spec.Warnings(); len(w) != 0 {
		t.Errorf("warnings = %q, want none at %d values", w, enumWarnThreshold)
	}
}

// A warning must name where the enum is, including inside a tool or a nested
// object: "an enum is large" is not actionable.
func TestWarnings_NameTheirPlace(t *testing.T) {
	big := make([]string, enumWarnThreshold+1)
	for i := range big {
		big[i] = fmt.Sprintf("p%d", i)
	}
	path := writeVocabulary(t, `
identity:
  name: classifier
  provider: ollama
  model: test-model
  prompt: "classify"
output:
  schema:
    type: object
    properties:
      places:
        type: array
        items: { type: string, enum: !include ./people.txt }
capabilities:
  tools:
    - name: lookup
      description: "looks a person up"
      command: ./lookup.py
      risk: none
      schema:
        type: object
        properties:
          person: { type: string, enum: !include ./people.txt }
`, "people.txt", strings.Join(big, "\n")+"\n")

	spec, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	warnings := strings.Join(spec.Warnings(), "\n")
	for _, want := range []string{"output.schema.places[].enum", "capabilities.tools[0].schema.person.enum"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("warnings lack %q:\n%s", want, warnings)
		}
	}
}

// The example in _examples must keep loading: a vocabulary that only works in
// a test is a vocabulary that does not work.
func TestWarnings_NoneOnTheExamples(t *testing.T) {
	spec, err := LoadManifest("../_examples/manifest.yaml")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if w := spec.Warnings(); len(w) != 0 {
		t.Errorf("warnings = %q, want none", w)
	}
}
