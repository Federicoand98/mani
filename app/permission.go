package app

import (
	"fmt"

	"github.com/Federicoand98/mani/core"
)

type Decision int

const (
	Deny Decision = iota
	AllowOnce
	AllowAlways
)

type PermissionManager struct {
	emit          func(PermissionRequestPayload)
	alwaysAllowed map[string]bool // sessione, non persistente
}

func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		alwaysAllowed: make(map[string]bool),
	}
}

func (m *PermissionManager) setEmit(emit func(PermissionRequestPayload)) {
	m.emit = emit
}

func (m *PermissionManager) check(toolName string, level core.RiskLevel, input map[string]any) error {
	if level == core.RiskNone || m.alwaysAllowed[toolName] {
		return nil
	}

	if m.emit == nil {
		return fmt.Errorf("no emit function set")
	}

	respond := make(chan Decision, 1)

	m.emit(PermissionRequestPayload{
		ToolName:  toolName,
		RiskLevel: level.String(),
		Input:     input,
	})

	switch <-respond {
	case AllowAlways:
		m.alwaysAllowed[toolName] = true
		return nil
	case AllowOnce:
		return nil
	default:
		return fmt.Errorf("permission denied from user")
	}
}

func (m *PermissionManager) Hook() core.PreToolUseHook {
	return func(name string, level core.RiskLevel, input map[string]any) error {
		return m.check(name, level, input)
	}
}
