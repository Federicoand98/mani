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

func RegisterGuardrails(rt *Runtime, spec GuardrailSpec) {
	var deny []denyRule

	for _, d := range spec.Deny {
		label := d.Label
		if label == "" {
			label = d.Pattern
		}
		deny = append(deny, denyRule{tool: d.Tool, re: regexp.MustCompile(d.Pattern), label: label})
	}

	var mask []maskRule
	for _, m := range spec.Mask {
		mask = append(mask, maskRule{re: regexp.MustCompile(m.Pattern), with: m.With})
	}

	if len(deny) > 0 {
		rt.OnPreToolUse(func(ctx context.Context, p *core.PreToolUsePayload) error {
			in := serializeInput(p.Input)
			for _, d := range deny {
				if d.tool == p.ToolName && d.re.MatchString(in) {
					return fmt.Errorf("guardrail: command blocked (%s)", d.label)
				}
			}
			return nil
		})
	}

	if len(mask) > 0 {
		rt.OnPostToolUse(func(ctx context.Context, p *core.PostToolUsePayload) error {
			for _, m := range mask {
				p.Result = m.re.ReplaceAllString(p.Result, m.with)
			}
			return nil
		})
	}
}

func serializeInput(in map[string]any) string {
	b, _ := json.Marshal(in)
	return string(b)
}
