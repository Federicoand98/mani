package app

import "github.com/Federicoand98/mani/core"

type PermissionHook func(toolName string, riskLevel string) error

func (r *Runtime) AddPermissionHook(h PermissionHook) *Runtime {
	r.agent.AddPreToolUseHook(func(name string, level core.RiskLevel) error {
		return h(name, level.String())
	})
	return r
}
