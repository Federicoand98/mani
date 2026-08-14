package app

import "github.com/Federicoand98/mani/core"

const (
	HookSessionStart core.HookType = "session_start"
	HookSessionEnd   core.HookType = "session_end"
)

type SessionEventPayload struct {
	SessionID string
	Title     string
}
