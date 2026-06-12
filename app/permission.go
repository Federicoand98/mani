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

type Prompter func(toolName string, riskLevel string, input map[string]any) Decision

type PermissionManager struct {
	prompter      Prompter
	alwaysAllowed map[string]bool // sessione, non persistente
}

func NewPermissionManager(prompter Prompter) *PermissionManager {
	return &PermissionManager{
		prompter:      prompter,
		alwaysAllowed: make(map[string]bool),
	}
}

func (m *PermissionManager) check(toolName string, level core.RiskLevel, input map[string]any) error {
	if level == core.RiskNone {
		return nil
	}

	if m.alwaysAllowed[toolName] {
		return nil
	}

	switch m.prompter(toolName, level.String(), input) {
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
