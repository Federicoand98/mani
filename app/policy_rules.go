package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

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

func RegisterNetworkPolicy(rt *Runtime, spec NetworkSpec) {
	if len(spec.Allow) == 0 && len(spec.Deny) == 0 {
		return
	}

	rt.agent.AddPreToolUseHook(func(ctx context.Context, toolName string, level core.RiskLevel, input map[string]any) error {
		if level != core.RiskNetwork {
			return nil
		}

		raw, _ := input["url"].(string)
		if raw == "" {
			return fmt.Errorf("[network policy]: tool %q has risk 'network' but no 'url' input to check", toolName)
		}

		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("[network policy]: invalid url %q: %w", raw, err)
		}

		host := strings.ToLower(u.Hostname())

		for _, p := range spec.Deny {
			if matchHost(p, host) {
				return fmt.Errorf("[network policy]: host %q is denied", host)
			}
		}

		if len(spec.Allow) == 0 {
			return nil
		}

		for _, p := range spec.Allow {
			if matchHost(p, host) {
				return nil
			}
		}

		return fmt.Errorf("[network policy]: host %q is not in policy.network.allow", host)
	})
}

func serializeInput(in map[string]any) string {
	b, _ := json.Marshal(in)
	return string(b)
}

func matchHost(pattern, host string) bool {
	pattern = strings.ToLower(pattern)
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(host, pattern[1:])
	}
	return pattern == host
}

func hostAllowedFunc(spec NetworkSpec) func(string) bool {
	if len(spec.Allow) == 0 && len(spec.Deny) == 0 {
		return nil
	}
	return func(host string) bool {
		host = strings.ToLower(host)
		for _, p := range spec.Deny {
			if matchHost(p, host) {
				return false
			}
		}
		if len(spec.Allow) == 0 {
			return true
		}
		for _, p := range spec.Allow {
			if matchHost(p, host) {
				return true
			}
		}
		return false
	}
}
