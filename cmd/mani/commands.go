package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/Federicoand98/mani/app"
)

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

var commands = []command{
	{"run", "run an agent from manifest (single task or trigger deamon)", runFromManifest},
	{"serve", "expose an agent over HTTP/websocket", runServer},
	{"init", "scaffold a new agent manifest in the current directory", runInit},
	{"validate", "check a manifest without running it", runValidate},
	{"runs", "list or inspect past runs from the journal", runRuns},
	{"tui", "start the interactive terminal chat", runTUICommand},
}

func lookupCommand(name string) (command, bool) {
	for _, cmd := range commands {
		if cmd.name == name {
			return cmd, true
		}
	}
	return command{}, false
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `mani - a declarative agent runtime

Usage:
  mani [command] [flags]
  mani						start the interactive terminal chat

Commands:
`)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range commands {
		fmt.Fprintf(tw, "  %s\t%s\n", c.name, c.summary)
	}
	tw.Flush()

	fmt.Fprint(w, `
Flags:
  -h, --help                show this help
  -v, --version             show the version

Run "mani <command> --help" for the flags of a single command.
`)
}

// commands
//

//go:embed templates/*.yaml
var templatesFS embed.FS

func templateNames() []string {
	entries, _ := templatesFS.ReadDir("templates")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names
}

func runInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	tmpl := fs.String("template", "agent", "template to scaffold: "+strings.Join(templateNames(), ", "))
	out := fs.String("o", "agent.yaml", "output file")
	force := fs.Bool("force", false, "overwrite the output file if it already exists")
	_ = fs.Parse(args)

	data, err := templatesFS.ReadFile("templates/" + *tmpl + ".yaml")
	if err != nil {
		return usagef("unknown template %q (available: %s)", *tmpl, strings.Join(templateNames(), ", "))
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if *force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(*out, flags, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return usagef("%s already exists (use --force to overwrite)", *out)
		}
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}

	abs, _ := filepath.Abs(*out)
	fmt.Printf("created %s\n\nnext:\n  mani validate --config %s\n  mani run --config %s --task \"hello\"\n",
		abs, *out, *out)
	return nil
}

func runValidate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the YAML manifest")
	_ = fs.Parse(args)

	if *configPath == "" {
		return usagef("--config is required")
	}

	spec, err := app.LoadManifest(*configPath)
	if err != nil {
		return usagef("%v", err)
	}

	fmt.Printf("%s: ok\n", *configPath)
	fmt.Printf("  identity:     %s (%s / %s)\n", spec.Identity.Name, spec.Identity.Provider, spec.Identity.Model)
	fmt.Printf("  tools:        %d\n", len(spec.Capabilities.Tools))
	fmt.Printf("  subagents:    %d\n", len(spec.Capabilities.Subagents))
	fmt.Printf("  triggers:     %d\n", len(spec.Run.Triggers))
	if spec.Output.Schema.Type != "" {
		fmt.Printf("  output:       structured\n")
	}
	return nil
}

func runRuns(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("runs", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the YAML manifest")
	path := fs.String("path", "", "journal directory (overrides the manifest)")
	limit := fs.Int("limit", 20, "maximum number of runs to list")
	status := fs.String("status", "", "filter by run status (ok, error, cancelled)")
	since := fs.String("since", "", "filter by runs since the given duration (e.g. 1h, 30m)")
	asJSON := fs.Bool("json", false, "output as JSON instead of table")
	_ = fs.Parse(args)

	j, err := openJournal(*configPath, *path)
	if err != nil {
		return err
	}

	if id := fs.Arg(0); id != "" {
		full, err := resolveRunID(j, id)
		if err != nil {
			return err
		}
		rec, err := j.Get(full)
		if err != nil {
			return fmt.Errorf("run %s: %w", id, err)
		}
		return printRun(os.Stdout, rec, *asJSON)
	}

	f := app.ListFilter{Limit: *limit, Status: *status}
	if *since != "" {
		d, err := time.ParseDuration(*since)
		if err != nil {
			return usagef("--since: %v", err)
		}
		f.Since = time.Now().Add(-d)
	}

	metas, err := j.List(f)
	if err != nil {
		return err
	}

	return printRuns(os.Stdout, metas, *asJSON)
}

func openJournal(configPath, dir string) (app.Journal, error) {
	if dir != "" {
		return app.NewJSONLJournal(dir)
	}

	if configPath == "" {
		return nil, usagef("--config or --path is required")
	}

	spec, err := app.LoadManifest(configPath)
	if err != nil {
		return nil, usagef("%v", err)
	}

	j, ok := app.OpenJournalReader(spec)
	if !ok {
		return nil, usagef("journal not enabled in the manifest (set observability.journal.path)")
	}

	return j, nil
}

func printRuns(w io.Writer, metas []app.RunMeta, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(metas)
	}

	if len(metas) == 0 {
		fmt.Fprintln(w, "no runs found")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tSTARTED\tDURATION\tTOKENS\tTOOLS\tBLOCKED")

	for _, m := range metas {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\t%d\t%d\n",
			m.ID,
			statusOrRunning(m),
			m.StartedAt.Local().Format("2006-01-02 15:04:05"),
			duration(m.StartedAt, m.EndedAt),
			m.Summary.InTokens, m.Summary.OutTokens,
			m.Summary.ToolCalls,
			m.Summary.Blocked,
		)
	}
	return tw.Flush()
}

func statusOrRunning(m app.RunMeta) string {
	if m.Status == "" {
		return "running"
	}
	return m.Status
}

func duration(start, end time.Time) string {
	if end.IsZero() {
		return "-"
	}
	return end.Sub(start).Round(100 * time.Millisecond).String()
}

func printRun(w io.Writer, rec app.RunRecord, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(rec)
	}

	fmt.Fprintf(w, "run %s  %s  %s → %s (%s)\n",
		rec.ID, statusOrRunning(rec.Meta()),
		rec.StartedAt.Local().Format("2006-01-02 15:04:05"),
		endClock(rec.EndedAt), duration(rec.StartedAt, rec.EndedAt))

	fmt.Fprintf(w, "source: %s   tokens: %d in / %d out   tools: %d   blocked: %d   masked: %d\n\n",
		rec.Source, rec.Summary.InTokens, rec.Summary.OutTokens,
		rec.Summary.ToolCalls, rec.Summary.Blocked, rec.Summary.Masked)

	for _, ev := range rec.Events {
		fmt.Fprintf(w, "%s  %s%-14s %s\n",
			ev.At.Local().Format("15:04:05.0"),
			indent(ev.Depth),
			ev.Kind,
			describe(ev))
	}
	return nil
}

func endClock(t time.Time) string {
	if t.IsZero() {
		return "…"
	}
	return t.Local().Format("15:04:05")
}

func indent(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("   ", depth-1) + "|- "
}

func describe(ev app.RunEvent) string {
	switch ev.Kind {
	case app.EvRunStart:
		return str(ev.Data, "source")
	case app.EvLLMCall:
		return fmt.Sprintf("messages=%d tools=%d", num(ev.Data, "messages"), num(ev.Data, "tools"))
	case app.EvLLMReponse:
		return fmt.Sprintf("stop=%s in=%d out=%d",
			str(ev.Data, "stop_reason"), num(ev.Data, "in_tokens"), num(ev.Data, "out_tokens"))
	case app.EvToolCall:
		return fmt.Sprintf("%-10s %s", str(ev.Data, "tool"), truncate(compact(ev.Data["input"]), 90))
	case app.EvToolResult:
		outcome := "ok"
		if b, _ := ev.Data["is_error"].(bool); b {
			outcome = "ERROR"
		}
		return fmt.Sprintf("%-10s %s  %d bytes", str(ev.Data, "tool"), outcome, num(ev.Data, "result_len"))
	case app.EvGuardrail:
		return fmt.Sprintf("%-10s %s  %q", str(ev.Data, "tool"), str(ev.Data, "action"), str(ev.Data, "label"))
	case app.EvRunEnd:
		return str(ev.Data, "status")
	}
	return compact(ev.Data)
}

func str(d map[string]any, k string) string {
	s, _ := d[k].(string)
	return s
}

func num(d map[string]any, k string) int {
	switch v := d[k].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return 0
}

func compact(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func resolveRunID(j app.Journal, prefix string) (string, error) {
	if _, err := j.Get(prefix); err == nil {
		return prefix, nil // id completo
	}

	metas, err := j.List(app.ListFilter{})
	if err != nil {
		return "", err
	}

	var hits []string
	for _, m := range metas {
		if strings.HasPrefix(m.ID, prefix) {
			hits = append(hits, m.ID)
		}
	}

	switch len(hits) {
	case 0:
		return "", fmt.Errorf("no run matches %q", prefix)
	case 1:
		return hits[0], nil
	default:
		return "", fmt.Errorf("%q is ambiguous: %s", prefix, strings.Join(hits, ", "))
	}
}
