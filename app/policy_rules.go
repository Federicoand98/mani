package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/Federicoand98/mani/core"
)

type denyRule struct {
	tool  string
	re    *regexp.Regexp
	label string
}

type maskRule struct {
	re   *regexp.Regexp
	with string
}

func RegisterPolicyRules(rt *Runtime, spec PolicySpec) {
	var deny []denyRule
	for _, r := range spec.Rules {
		if r.Action != "" && r.Action != "deny" {
			continue // slot per azioni future (warn, ask): oggi solo deny
		}
		label := r.Label
		if label == "" {
			label = r.Pattern
		}
		deny = append(deny, denyRule{tool: r.Tool, re: regexp.MustCompile(r.Pattern), label: label})
	}

	var mask []maskRule
	for _, m := range spec.Redact {
		mask = append(mask, maskRule{re: regexp.MustCompile(m.Pattern), with: m.With})
	}

	if len(deny) > 0 {
		rt.OnPreToolUse(func(ctx context.Context, p *core.PreToolUsePayload) error {
			in := serializeInput(p.Input)
			for _, d := range deny {
				if d.tool == p.ToolName && d.re.MatchString(in) {
					rt.recordGovernance(ctx, "deny", p.ToolName, d.label)
					return fmt.Errorf("guardrail: command blocked (%s)", d.label)
				}
			}
			return nil
		})
	}

	if len(mask) > 0 {
		rt.OnPostToolUse(func(ctx context.Context, p *core.PostToolUsePayload) error {
			masked := false
			for _, m := range mask {
				if m.re.MatchString(p.Result) {
					p.Result = m.re.ReplaceAllString(p.Result, m.with)
					masked = true
				}
			}

			if masked {
				rt.recordGovernance(ctx, "mask", p.ToolName, "")
			}

			return nil
		})
	}
}

func serializeInput(in map[string]any) string {
	b, _ := json.Marshal(in)
	return string(b)
}
