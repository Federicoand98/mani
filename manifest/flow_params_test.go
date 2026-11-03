package manifest

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParams_ShortAndLongFormKeepTheirOrder(t *testing.T) {
	var f struct {
		Params Params `yaml:"params"`
	}
	src := `
params:
  busta: "The busta to rebuild"
  since: { about: "Earliest year", default: "1490" }
  lang: { about: "Language of the letters" }
`
	if err := yaml.Unmarshal([]byte(src), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Params) != 3 || f.Params[0].Name != "busta" || f.Params[1].Name != "since" || f.Params[2].Name != "lang" {
		t.Fatalf("params = %+v, want busta, since, lang in that order", f.Params)
	}
	if f.Params[0].About != "The busta to rebuild" || f.Params[0].HasDefault {
		t.Errorf("busta = %+v, want the short form: a description and no default", f.Params[0])
	}
	if f.Params[1].Default != "1490" || !f.Params[1].HasDefault {
		t.Errorf("since = %+v, want default 1490", f.Params[1])
	}
	if f.Params[2].HasDefault {
		t.Errorf("lang = %+v, want no default", f.Params[2])
	}
}

func TestParams_UnknownKeyInsideAParamNamesItsLine(t *testing.T) {
	var f struct {
		Params Params `yaml:"params"`
	}
	err := yaml.Unmarshal([]byte("params:\n  busta:\n    about: x\n    defualt: 12\n"), &f)
	if err == nil || !strings.Contains(err.Error(), "line 4") || !strings.Contains(err.Error(), "defualt") {
		t.Errorf("err = %v, want the misspelt key and its line", err)
	}
}

const paramsFlow = `
flow: letters
about: "rebuilds a busta"
params:
  busta: "The busta to rebuild"
  since: { about: "Earliest year", default: "1490" }
steps:
  - step: fetch
    does: "downloads busta ${params.busta}"
    run: python fetch.py --busta ${params.busta} --since ${params.since}
result: fetch
`

func loadParamsFlow(t *testing.T, body string, set map[string]string) (Flow, error) {
	t.Helper()
	dir := writeFlowDir(t, map[string]string{"f.yaml": body})
	return LoadFlow(filepath.Join(dir, "f.yaml"), set)
}

func TestLoadFlow_ParamsFillTheValues(t *testing.T) {
	f, err := loadParamsFlow(t, paramsFlow, map[string]string{"busta": "12"})
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	want := []string{"python", "fetch.py", "--busta", "12", "--since", "1490"}
	if strings.Join(f.Steps[0].Run, " ") != strings.Join(want, " ") {
		t.Errorf("run = %q, want %q", f.Steps[0].Run, want)
	}
	if f.Steps[0].Does != "downloads busta 12" {
		t.Errorf("does = %q: parameters fill every value, not only commands", f.Steps[0].Does)
	}
	if got := f.Params[0]; got.Value != "12" || got.Origin != "--set" {
		t.Errorf("busta = %+v, want 12 from --set", got)
	}
	if got := f.Params[1]; got.Value != "1490" || got.Origin != "default" {
		t.Errorf("since = %+v, want 1490 from the default", got)
	}
	if m := f.MissingParams(); len(m) != 0 {
		t.Errorf("missing = %+v, want none", m)
	}
}

func TestLoadFlow_SetWinsOverTheDefault(t *testing.T) {
	f, err := loadParamsFlow(t, paramsFlow, map[string]string{"busta": "12", "since": "1501"})
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	if got := f.Params[1]; got.Value != "1501" || got.Origin != "--set" {
		t.Errorf("since = %+v, want 1501 from --set", got)
	}
}

// A required parameter with no value is not a load error — `mani validate`
// must be able to show the flow — but running is refused, with the fix in the
// message.
func TestLoadFlow_MissingParamIsReportedNotFatal(t *testing.T) {
	f, err := loadParamsFlow(t, paramsFlow, nil)
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	missing := f.MissingParams()
	if len(missing) != 1 || missing[0].Name != "busta" {
		t.Fatalf("missing = %+v, want busta", missing)
	}
	if !strings.Contains(strings.Join(f.Steps[0].Run, " "), "${params.busta}") {
		t.Errorf("run = %q, want the unset reference left as written", f.Steps[0].Run)
	}
	err = f.CheckParams()
	for _, want := range []string{"busta", "The busta to rebuild", "--set busta="} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckParams = %v, want it to contain %q", err, want)
		}
	}
}

func TestLoadFlow_UnknownParamReferenceNamesItsLine(t *testing.T) {
	body := strings.Replace(paramsFlow, "--busta ${params.busta}", "--busta ${params.bsta}", 1)
	_, err := loadParamsFlow(t, body, map[string]string{"busta": "12"})
	for _, want := range []string{"line 10", `"bsta"`, "busta, since"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to contain %q", err, want)
		}
	}
}

// A --set nobody declared is a typo; ignoring it is what declarations exist
// to prevent.
func TestLoadFlow_SetOfAnUndeclaredParamIsAnError(t *testing.T) {
	_, err := loadParamsFlow(t, paramsFlow, map[string]string{"busta": "12", "bust": "13"})
	if err == nil || !strings.Contains(err.Error(), "--set bust") || !strings.Contains(err.Error(), "busta, since") {
		t.Errorf("err = %v, want the stray name and the declared ones", err)
	}
}

// ${VAR} and ${params.x} coexist in one value: the env pass leaves the dotted
// form alone, the params pass fills it.
func TestLoadFlow_ParamsAndEnvDoNotCollide(t *testing.T) {
	t.Setenv("ARCHIVE_URL", "https://example.org")
	body := strings.Replace(paramsFlow, "python fetch.py", "python fetch.py --from ${ARCHIVE_URL}", 1)
	f, err := loadParamsFlow(t, body, map[string]string{"busta": "12"})
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	got := strings.Join(f.Steps[0].Run, " ")
	if !strings.Contains(got, "--from https://example.org") || !strings.Contains(got, "--busta 12") {
		t.Errorf("run = %q, want both expanded", got)
	}
}

// Same rule as ${VAR}: a block scalar is prose and is left alone.
func TestLoadFlow_ParamsAreNotExpandedInBlockScalars(t *testing.T) {
	body := strings.Replace(paramsFlow, `does: "downloads busta ${params.busta}"`, "does: |\n      downloads busta ${params.busta}", 1)
	f, err := loadParamsFlow(t, body, map[string]string{"busta": "12"})
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	if !strings.Contains(f.Steps[0].Does, "${params.busta}") {
		t.Errorf("does = %q, want the block scalar untouched", f.Steps[0].Does)
	}
}

func TestLoadFlow_AnAgentPathNeedingAMissingParamNamesTheStep(t *testing.T) {
	dir := writeFlowDir(t, map[string]string{"f.yaml": `
flow: letters
about: "x"
params:
  model: "which extractor"
input: "letters"
steps:
  - step: extract
    does: "extracts"
    agent: extract-${params.model}.yaml
    for_each: input
result: extract
`})
	_, err := LoadFlow(filepath.Join(dir, "f.yaml"), nil)
	if err == nil || !strings.Contains(err.Error(), "step extract") || !strings.Contains(err.Error(), "no value") {
		t.Errorf("err = %v, want the step and the reason", err)
	}
}

func TestParams_Validate(t *testing.T) {
	cases := []struct {
		name string
		p    Params
		want string
	}{
		{"name with a capital", Params{{Name: "Busta", About: "x"}}, `"Busta" is not a name`},
		{"name with a dash", Params{{Name: "bu-sta", About: "x"}}, "is not a name"},
		{"no description", Params{{Name: "busta", About: " "}}, "say in a few words"},
		{"declared twice", Params{{Name: "busta", About: "x"}, {Name: "busta", About: "y"}}, "declared twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("validate = %v, want %q", err, tc.want)
			}
		})
	}
}
