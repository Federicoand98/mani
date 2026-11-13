package manifest

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Problem is one thing wrong with a manifest: the field it is about, the line
// of the file where that field is, and what is wrong.
type Problem struct {
	Path    string // e.g. "policy.rules[1].pattern"; empty for a problem with the whole file
	Line    int    // 0 when unknown
	Message string
}

func (p Problem) Error() string { return "[manifest]: " + p.Message }

// Check parses and validates a manifest held in memory, reporting every
// problem with its line instead of stopping at the first. baseDir resolves
// !include. The Spec is usable only when there are no problems.
func Check(data []byte, baseDir string) (Spec, []Problem) {
	spec, written, err := parse(data, baseDir, "")
	if err != nil {
		return spec, parseProblems(err)
	}
	ps := spec.Problems()
	for i := range ps {
		ps[i].Line = lineAt(written, ps[i].Path)
	}
	return spec, ps
}

var lineRe = regexp.MustCompile(`line (\d+)`)

// parseProblems turns a decoding error into problems: yaml.v3 reports every
// unknown field and wrong type at once, each with its line.
func parseProblems(err error) []Problem {
	var te *yaml.TypeError
	if errors.As(err, &te) {
		out := make([]Problem, 0, len(te.Errors))
		for _, e := range te.Errors {
			out = append(out, Problem{Line: lineOf(e), Message: e})
		}
		return out
	}
	msg := strings.TrimPrefix(err.Error(), "[manifest]: ")
	return []Problem{{Line: lineOf(msg), Message: msg}}
}

func lineOf(s string) int {
	if m := lineRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// lineAt finds the line of a field path in the tree as written. When the
// field itself is absent (a required one, say), it answers with the closest
// enclosing field that exists.
func lineAt(doc *yaml.Node, path string) int {
	if doc == nil || len(doc.Content) == 0 {
		return 0
	}
	node, line := doc.Content[0], doc.Content[0].Line
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			continue
		}
		key, idx := seg, -1
		if i := strings.IndexByte(seg, '['); i >= 0 && strings.HasSuffix(seg, "]") {
			key = seg[:i]
			idx, _ = strconv.Atoi(seg[i+1 : len(seg)-1])
		}
		if key != "" {
			next := mapValue(node, key)
			if next == nil {
				return line
			}
			line = next.key.Line
			node = next.value
		}
		if idx >= 0 {
			if node.Kind != yaml.SequenceNode || idx >= len(node.Content) {
				return line
			}
			node = node.Content[idx]
			line = node.Line
		}
	}
	return line
}

type pair struct{ key, value *yaml.Node }

func mapValue(n *yaml.Node, key string) *pair {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return &pair{n.Content[i], n.Content[i+1]}
		}
	}
	return nil
}

// Problems lists everything wrong with the manifest, in file order of the
// blocks. The messages are those Validate has always returned.
func (s Spec) Problems() []Problem {
	var out []Problem
	add := func(path, format string, args ...any) {
		out = append(out, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	// --- capabilities.tools ---
	declared := map[string]bool{}
	for i, ref := range s.Capabilities.Tools {
		at := fmt.Sprintf("capabilities.tools[%d]", i)
		if ref.Name == "" {
			add(at, "capabilities.tools: tool name required")
			continue
		}
		if declared[ref.Name] {
			add(at+".name", "tool %q declarated too many times", ref.Name)
		}
		declared[ref.Name] = true

		if ref.IsSubprocess() {
			if ref.Schema.Type == "" {
				add(at, "tool subprocess %q requires schema.type", ref.Name)
			}
		} else if !IsBuiltinTool(ref.Name) {
			add(at, "tool unknown %q (built-in: %s)", ref.Name, strings.Join(BuiltinTools(), ", "))
		}
	}

	if len(s.Capabilities.Subagents) > 0 && !declared["delegate"] {
		add("capabilities.subagents", "capabilities.subagents but \"delegate\" tool not declared")
	}

	seen := map[string]bool{}
	for i, sa := range s.Capabilities.Subagents {
		at := fmt.Sprintf("capabilities.subagents[%d]", i)
		if sa.Name == "" {
			add(at, "subagent: name required")
			continue
		}
		if sa.Description == "" {
			add(at, "subagent %q: description required", sa.Name)
		}
		if seen[sa.Name] {
			add(at+".name", "subagent %q: duplicate", sa.Name)
		}
		seen[sa.Name] = true
		for _, t := range sa.Tools {
			if !IsBuiltinTool(t) && !declared[t] {
				add(at+".tools", "subagent %q: tool unknown %q", sa.Name, t)
			}
		}
	}

	// --- policy ---
	names := make([]string, 0, len(s.Policy.Tools))
	for name := range s.Policy.Tools {
		names = append(names, name)
	}
	sort.Strings(names) // a map has no order; the report must have one
	for _, name := range names {
		switch p := s.Policy.Tools[name]; p {
		case RiskPolicyAllow, RiskPolicyAsk, RiskPolicyDeny:
		default:
			add("policy.tools."+name, "policy.tools[%s]: invalid value %q (allow|ask|deny)", name, p)
		}
	}
	for i, r := range s.Policy.Rules {
		at := fmt.Sprintf("policy.rules[%d]", i)
		if r.Pattern == "" {
			add(at, "policy.rules[%d]: pattern required", i)
		} else if _, err := regexp.Compile(r.Pattern); err != nil {
			add(at+".pattern", "policy.rules[%d]: invalid regex: %v", i, err)
		}
		if r.Action != "" && r.Action != "deny" {
			add(at+".action", "policy.rules[%d]: action %q not supported (only \"deny\")", i, r.Action)
		}
	}
	for i, m := range s.Policy.Redact {
		if _, err := regexp.Compile(m.Pattern); err != nil {
			add(fmt.Sprintf("policy.redact[%d].pattern", i), "policy.redact[%d]: invalid regex: %v", i, err)
		}
	}

	// --- output ---
	if s.Output.Schema.Type != "" && s.Output.Schema.Type != "object" {
		add("output.schema.type", "output.schema.type must be \"object\", found %q", s.Output.Schema.Type)
	}

	// --- limits (durations) ---
	for _, d := range []struct{ path, value string }{
		{"limits.max_duration", s.Limits.MaxDuration},
		{"limits.tool_timeout", s.Limits.ToolTimeout},
	} {
		if d.value == "" {
			continue
		}
		if _, err := time.ParseDuration(d.value); err != nil {
			add(d.path, "%s: %v", d.path, err)
		}
	}

	// --- run ---
	if s.Run.Scheduler.Concurrency < 0 {
		add("run.scheduler.concurrency", "run.scheduler.concurrency must be >= 0")
	}
	if b := s.Run.Scheduler.Retry.Backoff; b != "" {
		if _, err := time.ParseDuration(b); err != nil {
			add("run.scheduler.retry.backoff", "run.scheduler.retry.backoff: %v", err)
		}
	}
	seenTrigger := map[string]bool{}
	seenPath := map[string]bool{}
	webhookAddr := ""
	for i, t := range s.Run.Triggers {
		at := fmt.Sprintf("run.triggers[%d]", i)
		switch t.Type {
		case "every", "daily", "webhook":
		default:
			add(at+".type", "run.triggers[%d]: type %q not valid (every|daily|webhook)", i, t.Type)
		}
		if t.Memory != "" && t.Memory != "fresh" && t.Memory != "persistent" {
			add(at+".memory", "run.triggers[%d]: memory must be fresh|persistent, found %q", i, t.Memory)
		}
		if t.Type == "daily" {
			if _, _, err := ParseClock(t.At); err != nil {
				add(at+".at", "run.triggers[%d].at: %v", i, err)
			}
		}
		if t.Type == "webhook" {
			path := t.HookPath()
			if !strings.HasPrefix(path, "/") {
				add(at+".path", "run.triggers[%d].path: must start with \"/\", found %q", i, path)
			}
			if seenPath[path] {
				add(at+".path", "run.triggers[%d].path: %q declared twice", i, path)
			}
			seenPath[path] = true

			// there is one listener: two addresses would make a silent webhook
			if t.Addr != "" {
				if webhookAddr != "" && webhookAddr != t.Addr {
					add(at+".addr", "run.triggers: webhook triggers must share one addr, found %q and %q", webhookAddr, t.Addr)
				}
				webhookAddr = t.Addr
			}
		}
		if t.Name != "" {
			if seenTrigger[t.Name] {
				add(at+".name", "run.triggers: name %q duplicate", t.Name)
			}
			seenTrigger[t.Name] = true
		}
	}

	// --- observability ---
	switch backend := strings.ToLower(strings.TrimSpace(s.Observability.Journal.Backend)); backend {
	case "", "jsonl":
	case "sqlite":
		if s.Observability.Journal.Enabled && s.Observability.Journal.Path == "" {
			add("observability.journal", "observability.journal: sqlite backend requires path")
		}
	default:
		add("observability.journal.backend", "observability.journal.backend: unsupported backend %q (jsonl|sqlite)", s.Observability.Journal.Backend)
	}

	return out
}
