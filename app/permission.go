package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/Federicoand98/mani/core"
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
}

type Decision int

const (
	Deny Decision = iota
	AllowOnce
	AllowAlways
)

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
		switch d {
		case AllowAlways:
			m.mu.Lock()
			m.alwaysAllowed[toolName] = true
			m.mu.Unlock()
			return nil
		case AllowOnce:
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
