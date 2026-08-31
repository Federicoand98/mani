package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Federicoand98/mani/app"
)

// ---------------------------------------------------------------------------
// helper: un journal JSONL vero su disco, perche' e' quello che gli utenti hanno
// ---------------------------------------------------------------------------

var testStart = time.Date(2026, 8, 14, 18, 6, 10, 0, time.UTC)

// seedJournal scrive un journal su disco e restituisce la directory.
// Si usa il JSONL e non l'InMemory di proposito: e' l'unico che fa passare i
// dati per JSON, ed e' li' che i tipi cambiano sotto i piedi.
func seedJournal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	j, err := app.NewJSONLJournal(dir)
	if err != nil {
		t.Fatalf("NewJSONLJournal: %v", err)
	}

	// run 1: completata, con una tool call, un blocco e un subagent
	if err := j.Start(app.RunRecord{ID: "aaaa11112222", Source: "cli", SessionID: "s1", StartedAt: testStart}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	appendAll(t, j, "aaaa11112222",
		ev(app.EvLLMReponse, 0, testStart.Add(1*time.Second), map[string]any{
			"stop_reason": "tool_use", "in_tokens": 681, "out_tokens": 96,
		}),
		ev(app.EvToolCall, 0, testStart.Add(2*time.Second), map[string]any{
			"tool": "read", "input": map[string]any{"path": "incident.log"},
		}),
		ev(app.EvToolResult, 0, testStart.Add(2*time.Second), map[string]any{
			"tool": "read", "is_error": false, "result_len": 559,
		}),
		// il subagent: Depth 1 → deve comparire indentato
		ev(app.EvLLMCall, 1, testStart.Add(3*time.Second), map[string]any{
			"messages": 4, "tools": 2,
		}),
		ev(app.EvGuardrail, 0, testStart.Add(4*time.Second), map[string]any{
			"tool": "bash", "action": "deny", "label": "recursive delete",
		}),
	)
	if err := j.Finish("aaaa11112222", "ok"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// run 2: fallita, e piu' vecchia di un'ora (serve a --since)
	old := testStart.Add(-2 * time.Hour)
	if err := j.Start(app.RunRecord{ID: "bbbb33334444", Source: "trigger", StartedAt: old}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := j.Finish("bbbb33334444", "error"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	return dir
}

func ev(kind app.EventKind, depth int, at time.Time, data map[string]any) app.RunEvent {
	return app.RunEvent{Kind: kind, Depth: depth, At: at, Data: data}
}

func appendAll(t *testing.T, j app.Journal, runID string, evs ...app.RunEvent) {
	t.Helper()
	for _, e := range evs {
		e.RunID = runID
		if err := j.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
}

func writeManifest(t *testing.T, journalPath string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.yaml")

	body := "identity:\n" +
		"  name: t\n" +
		"  provider: ollama\n" +
		"  model: x\n" +
		"observability:\n" +
		"  journal:\n" +
		"    enabled: true\n" +
		"    path: " + journalPath + "\n"

	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// funzioni pure
// ---------------------------------------------------------------------------

func TestIndent_ByDepth(t *testing.T) {
	cases := map[int]string{
		0: "",
		1: "|- ",
		2: "   |- ",
		3: "      |- ",
	}
	for depth, want := range cases {
		if got := indent(depth); got != want {
			t.Errorf("indent(%d) = %q, want %q", depth, got, want)
		}
	}
	// depth negativo non deve panicare su strings.Repeat
	if got := indent(-1); got != "" {
		t.Errorf("indent(-1) = %q, want empty", got)
	}
}

// I numeri che attraversano il JSONL tornano come float64: encoding/json non
// conosce il tipo di destinazione. Un semplice d[k].(int) funzionerebbe con
// l'InMemoryJournal e restituirebbe silenziosamente 0 leggendo da disco — cioe'
// nel caso d'uso reale.
func TestNum_HandlesEveryNumericShape(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want int
	}{
		{"int", 42, 42},
		{"int64", int64(42), 42},
		{"float64 (da JSON)", float64(42), 42},
		{"json.Number", json.Number("42"), 42},
		{"assente", nil, 0},
		{"stringa", "42", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := map[string]any{}
			if c.val != nil {
				d["k"] = c.val
			}
			if got := num(d, "k"); got != c.want {
				t.Errorf("num = %d, want %d", got, c.want)
			}
		})
	}
}

func TestTruncate_CutsOnRuneBoundary(t *testing.T) {
	s := strings.Repeat("è", 60) // 2 byte per rune
	got := truncate(s, 25)

	if !strings.HasSuffix(got, "…") {
		t.Errorf("manca il marcatore di troncamento: %q", got)
	}
	if !utf8ValidString(got) {
		t.Errorf("troncamento a meta' rune: %q", got)
	}
	if truncate("corto", 25) != "corto" {
		t.Error("una stringa sotto il limite non deve essere toccata")
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

func TestDuration_RunningRunHasNoDuration(t *testing.T) {
	if got := duration(testStart, time.Time{}); got != "-" {
		t.Errorf("run in corso: durata %q, want %q", got, "-")
	}
	if got := duration(testStart, testStart.Add(3600*time.Millisecond)); got != "3.6s" {
		t.Errorf("durata = %q, want 3.6s", got)
	}
}

func TestStatusOrRunning(t *testing.T) {
	if got := statusOrRunning(app.RunMeta{Status: ""}); got != "running" {
		t.Errorf("status vuoto = %q, want running", got)
	}
	if got := statusOrRunning(app.RunMeta{Status: "ok"}); got != "ok" {
		t.Errorf("status = %q, want ok", got)
	}
}

func TestDescribe_PerKind(t *testing.T) {
	cases := []struct {
		kind     app.EventKind
		data     map[string]any
		contains []string
	}{
		{app.EvRunStart, map[string]any{"source": "cli"}, []string{"cli"}},
		{app.EvLLMCall, map[string]any{"messages": float64(4), "tools": float64(2)}, []string{"messages=4", "tools=2"}},
		{app.EvLLMReponse, map[string]any{"stop_reason": "tool_use", "in_tokens": float64(681), "out_tokens": float64(96)},
			[]string{"stop=tool_use", "in=681", "out=96"}},
		{app.EvToolCall, map[string]any{"tool": "read", "input": map[string]any{"path": "x.log"}},
			[]string{"read", "x.log"}},
		{app.EvToolResult, map[string]any{"tool": "read", "is_error": false, "result_len": float64(559)},
			[]string{"read", "ok", "559"}},
		{app.EvToolResult, map[string]any{"tool": "bash", "is_error": true, "result_len": float64(12)},
			[]string{"bash", "ERROR"}},
		{app.EvGuardrail, map[string]any{"tool": "bash", "action": "deny", "label": "recursive delete"},
			[]string{"bash", "deny", "recursive delete"}},
		{app.EvRunEnd, map[string]any{"status": "ok"}, []string{"ok"}},
	}

	for _, c := range cases {
		t.Run(string(c.kind), func(t *testing.T) {
			got := describe(app.RunEvent{Kind: c.kind, Data: c.data})
			for _, want := range c.contains {
				if !strings.Contains(got, want) {
					t.Errorf("describe(%s) = %q, manca %q", c.kind, got, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// output
// ---------------------------------------------------------------------------

// Un elenco vuoto va detto a parole: una tabella con la sola intestazione
// sembra un errore del comando, non una risposta.
func TestPrintRuns_EmptySaysSo(t *testing.T) {
	var b bytes.Buffer
	if err := printRuns(&b, nil, false); err != nil {
		t.Fatalf("printRuns: %v", err)
	}
	if !strings.Contains(b.String(), "no runs") {
		t.Errorf("output = %q, atteso un messaggio esplicito", b.String())
	}
	if strings.Contains(b.String(), "STATUS") {
		t.Error("con zero run non si stampa l'intestazione della tabella")
	}
}

func TestPrintRuns_Table(t *testing.T) {
	var b bytes.Buffer
	metas := []app.RunMeta{{
		ID: "aaaa11112222", Status: "ok",
		StartedAt: testStart, EndedAt: testStart.Add(3 * time.Second),
		Summary: app.Summary{InTokens: 681, OutTokens: 96, ToolCalls: 2, Blocked: 1},
	}}
	if err := printRuns(&b, metas, false); err != nil {
		t.Fatalf("printRuns: %v", err)
	}

	out := b.String()
	for _, want := range []string{"ID", "STATUS", "BLOCKED", "aaaa11112222", "ok", "681/96"} {
		if !strings.Contains(out, want) {
			t.Errorf("tabella priva di %q:\n%s", want, out)
		}
	}
}

func TestPrintRuns_JSONIsValid(t *testing.T) {
	var b bytes.Buffer
	metas := []app.RunMeta{{ID: "aaaa11112222", Status: "ok", StartedAt: testStart}}
	if err := printRuns(&b, metas, true); err != nil {
		t.Fatalf("printRuns: %v", err)
	}

	var back []app.RunMeta
	if err := json.Unmarshal(b.Bytes(), &back); err != nil {
		t.Fatalf("output non deserializzabile: %v\n%s", err, b.String())
	}
	if len(back) != 1 || back[0].ID != "aaaa11112222" {
		t.Errorf("round-trip perso: %+v", back)
	}
	if strings.Contains(b.String(), "STATUS") {
		t.Error("con --json non si stampa la tabella")
	}
}

// Il subagent (Depth 1) deve comparire indentato: e' l'unica cosa che rende
// leggibile una run con delegate, ed e' il motivo per cui Depth sta nel RunEvent.
func TestPrintRun_IndentsSubagentEvents(t *testing.T) {
	var b bytes.Buffer
	rec := app.RunRecord{
		ID: "aaaa11112222", Source: "cli", Status: "ok",
		StartedAt: testStart, EndedAt: testStart.Add(4 * time.Second),
		Events: []app.RunEvent{
			ev(app.EvToolCall, 0, testStart.Add(1*time.Second), map[string]any{"tool": "delegate", "input": map[string]any{"agent": "researcher"}}),
			ev(app.EvLLMCall, 1, testStart.Add(2*time.Second), map[string]any{"messages": 3, "tools": 1}),
		},
	}
	if err := printRun(&b, rec, false); err != nil {
		t.Fatalf("printRun: %v", err)
	}

	var parent, child string
	for _, line := range strings.Split(b.String(), "\n") {
		if strings.Contains(line, "tool_call") {
			parent = line
		}
		if strings.Contains(line, "llm_call") {
			child = line
		}
	}
	if parent == "" || child == "" {
		t.Fatalf("righe non trovate:\n%s", b.String())
	}
	if strings.Contains(parent, "|-") {
		t.Errorf("l'evento a depth 0 non va indentato: %q", parent)
	}
	if !strings.Contains(child, "|-") {
		t.Errorf("l'evento a depth 1 va indentato: %q", child)
	}
}

// ---------------------------------------------------------------------------
// apertura del journal
// ---------------------------------------------------------------------------

func TestOpenJournal_FromPath(t *testing.T) {
	dir := seedJournal(t)

	j, err := openJournal("", dir)
	if err != nil {
		t.Fatalf("openJournal: %v", err)
	}
	metas, err := j.List(app.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 2 {
		t.Errorf("run trovate %d, attese 2", len(metas))
	}
}

func TestOpenJournal_FromManifest(t *testing.T) {
	dir := seedJournal(t)
	manifest := writeManifest(t, dir)

	j, err := openJournal(manifest, "")
	if err != nil {
		t.Fatalf("openJournal: %v", err)
	}
	if _, err := j.Get("aaaa11112222"); err != nil {
		t.Errorf("Get dalla run seminata: %v", err)
	}
}

// --path vince sul manifest: serve a ispezionare la directory di run altrui
// senza avere il loro manifest.
func TestOpenJournal_PathOverridesManifest(t *testing.T) {
	seeded := seedJournal(t)
	manifest := writeManifest(t, t.TempDir()) // manifest che punta altrove

	j, err := openJournal(manifest, seeded)
	if err != nil {
		t.Fatalf("openJournal: %v", err)
	}
	metas, _ := j.List(app.ListFilter{})
	if len(metas) != 2 {
		t.Errorf("--path ignorato: run trovate %d, attese 2", len(metas))
	}
}

func TestOpenJournal_NoArgs_IsUsageError(t *testing.T) {
	_, err := openJournal("", "")
	if err == nil {
		t.Fatal("atteso errore senza --config e senza --path")
	}
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code %d, atteso %d (errore d'uso)", exitCodeFor(err), exitUsage)
	}
}

func TestOpenJournal_JournalNotEnabled_IsUsageError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.yaml")
	body := "identity:\n  name: t\n  provider: ollama\n  model: x\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := openJournal(p, "")
	if err == nil {
		t.Fatal("atteso errore con journal non abilitato")
	}
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code %d, atteso %d", exitCodeFor(err), exitUsage)
	}
	if !strings.Contains(err.Error(), "journal") {
		t.Errorf("il messaggio deve dire cosa manca: %v", err)
	}
}

// ---------------------------------------------------------------------------
// prefisso dell'id
// ---------------------------------------------------------------------------

func TestResolveRunID(t *testing.T) {
	dir := seedJournal(t)
	j, err := openJournal("", dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("id completo", func(t *testing.T) {
		got, err := resolveRunID(j, "aaaa11112222")
		if err != nil || got != "aaaa11112222" {
			t.Errorf("got %q, err %v", got, err)
		}
	})

	t.Run("prefisso univoco", func(t *testing.T) {
		got, err := resolveRunID(j, "aaaa")
		if err != nil || got != "aaaa11112222" {
			t.Errorf("got %q, err %v", got, err)
		}
	})

	t.Run("inesistente", func(t *testing.T) {
		if _, err := resolveRunID(j, "zzzz"); err == nil {
			t.Error("atteso errore per prefisso senza corrispondenze")
		}
	})
}

// Un prefisso ambiguo elenca i candidati invece di sceglierne uno: indovinare
// quale run volevi vedere e' peggio che chiedere.
func TestResolveRunID_AmbiguousListsCandidates(t *testing.T) {
	dir := t.TempDir()
	j, err := app.NewJSONLJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dup1", "dup2"} {
		if err := j.Start(app.RunRecord{ID: id, Source: "cli", StartedAt: testStart}); err != nil {
			t.Fatal(err)
		}
	}

	_, err = resolveRunID(j, "dup")
	if err == nil {
		t.Fatal("atteso errore per prefisso ambiguo")
	}
	for _, id := range []string{"dup1", "dup2"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("l'errore deve elencare i candidati, manca %q: %v", id, err)
		}
	}
}

// ---------------------------------------------------------------------------
// filtri (contratto del port, esercitato attraverso il JSONL)
// ---------------------------------------------------------------------------

func TestList_FilterByStatus(t *testing.T) {
	dir := seedJournal(t)
	j, _ := openJournal("", dir)

	metas, err := j.List(app.ListFilter{Status: "error"})
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != "bbbb33334444" {
		t.Errorf("filtro status: %+v", metas)
	}
}

func TestList_FilterBySince(t *testing.T) {
	dir := seedJournal(t)
	j, _ := openJournal("", dir)

	// la run vecchia e' 2h prima di testStart: con Since a 1h prima resta fuori
	metas, err := j.List(app.ListFilter{Since: testStart.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != "aaaa11112222" {
		t.Errorf("filtro since: %+v", metas)
	}
}

func TestList_LimitAppliesAfterSorting(t *testing.T) {
	dir := seedJournal(t)
	j, _ := openJournal("", dir)

	metas, err := j.List(app.ListFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 {
		t.Fatalf("limit ignorato: %d run", len(metas))
	}
	// la piu' recente e' la prima: se il limite fosse applicato prima
	// dell'ordinamento, qui uscirebbe una run a caso
	if metas[0].ID != "aaaa11112222" {
		t.Errorf("con limit=1 deve uscire la piu' recente, ottenuta %q", metas[0].ID)
	}
}

// REGRESSIONE: apply() confrontava Data["action"] con "denied"/"masked", mentre
// recordGovernance scrive "deny"/"mask". Il contatore Blocked restava a zero
// anche con la regola che scattava a ogni giro — e la colonna BLOCKED di
// `mani runs` mostrava 0 mentendo.
func TestSummary_CountsBlockedGuardrails(t *testing.T) {
	dir := seedJournal(t)
	j, _ := openJournal("", dir)

	rec, err := j.Get("aaaa11112222")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Summary.Blocked != 1 {
		t.Errorf("Blocked = %d, atteso 1 (un evento guardrail con action=deny)", rec.Summary.Blocked)
	}
}

func TestList_CorruptedFileIsSkipped(t *testing.T) {
	dir := seedJournal(t)
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	j, _ := openJournal("", dir)
	metas, err := j.List(app.ListFilter{})
	if err != nil {
		t.Fatalf("un file corrotto non deve far fallire l'elenco: %v", err)
	}
	if len(metas) != 2 {
		t.Errorf("run trovate %d, attese 2", len(metas))
	}
}
