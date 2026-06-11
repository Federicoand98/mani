package core

type RiskLevel int

const (
	RiskNone RiskLevel = iota
	RiskWrite
	RiskExecute
)

func (r RiskLevel) String() string {
	switch r {
	case RiskWrite:
		return "write"
	case RiskExecute:
		return "execute"
	default:
		return "none"
	}
}

type PreToolUseHook func(toolName string, level RiskLevel) error
