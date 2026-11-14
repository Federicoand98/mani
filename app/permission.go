package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/journal"
)

type permEmitKey struct{}

func WithPermissionEmit(ctx context.Context, emit func(PermissionRequestPayload)) context.Context {
	return context.WithValue(ctx, permEmitKey{}, emit)
}

func permissionEmitFrom(ctx context.Context) func(PermissionRequestPayload) {
	fn, _ := ctx.Value(permEmitKey{}).(func(PermissionRequestPayload))
	return fn
}

type PermissionManager struct {
	alwaysAllowed map[string]bool // sessione, non persistente
	mu            sync.Mutex      // per subagents
	// record, when set, is told every answer: the journal keeps it.
	record func(ctx context.Context, tool, decision string)
}

type Decision int

const (
	Deny Decision = iota
	AllowOnce
	AllowAlways
	// AllowRemembered: allowed without asking, because an earlier answer
	// covers this call (a tool allowed for the whole conversation).
	AllowRemembered
	// DenyExpired: someone could have answered and nobody did in time.
	DenyExpired
	// DenyUnattended: there is nobody to ask (a trigger, mani run, an API
	// call), so the call is denied without asking.
	DenyUnattended
)

// journalDecision is how the journal names d.
func journalDecision(d Decision) string {
	switch d {
	case AllowOnce:
		return journal.DecisionAllowed
	case AllowAlways:
		return journal.DecisionAllowedAlways
	case AllowRemembered:
		return journal.DecisionRemembered
	case DenyExpired:
		return journal.DecisionExpired
	case DenyUnattended:
		return journal.DecisionUnattended
	default:
		return journal.DecisionDenied
	}
}

func (m *PermissionManager) note(ctx context.Context, tool string, d Decision) {
	if m.record != nil {
		m.record(ctx, tool, journalDecision(d))
	}
}

func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		alwaysAllowed: make(map[string]bool),
	}
}

func (m *PermissionManager) check(ctx context.Context, toolName string, level core.RiskLevel, input map[string]any) error {
	if level == core.RiskNone {
		return nil
	}

	m.mu.Lock()
	allowed := m.alwaysAllowed[toolName]
	m.mu.Unlock()
	if allowed {
		m.note(ctx, toolName, AllowRemembered)
		return nil
	}

	emit := permissionEmitFrom(ctx)
	if emit == nil {
		return fmt.Errorf("[permission manager]: no emit function set")
	}

	respond := make(chan Decision, 1)
	emit(PermissionRequestPayload{
		ToolName:  toolName,
		RiskLevel: level.String(),
		Input:     input,
		Respond:   respond,
		Preview:   renderPreview(input),
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case d := <-respond:
		m.note(ctx, toolName, d)
		switch d {
		case AllowAlways:
			m.mu.Lock()
			m.alwaysAllowed[toolName] = true
			m.mu.Unlock()
			return nil
		case AllowOnce, AllowRemembered:
			return nil
		default:
			return fmt.Errorf("permission denied from user")
		}
	}
}

func (m *PermissionManager) Hook() core.PreToolUseHook {
	return func(ctx context.Context, name string, level core.RiskLevel, input map[string]any) error {
		return m.check(ctx, name, level, input)
	}
}
