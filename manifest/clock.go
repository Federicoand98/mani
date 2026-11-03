package manifest

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultHookPath is the route of a webhook trigger that declares no path.
const DefaultHookPath = "/hook"

// ParseClock reads an "HH:MM" wall-clock time in the local timezone.
//
// It is deliberately strict about trailing input: fmt.Sscanf ignores whatever
// follows the pattern, so "09:00 UTC" used to parse as 09:00 local, silently
// running the trigger at a time the manifest did not ask for.
func ParseClock(clock string) (int, int, error) {
	invalid := func() (int, int, error) {
		return 0, 0, fmt.Errorf("expected HH:MM in local time, found %q", clock)
	}

	hs, ms, ok := strings.Cut(clock, ":")
	if !ok || !isDigits(hs) || !isDigits(ms) {
		return invalid()
	}

	h, err := strconv.Atoi(hs)
	if err != nil {
		return invalid()
	}
	m, err := strconv.Atoi(ms)
	if err != nil {
		return invalid()
	}

	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("hour must be between 0 and 23, minute must be between 0 and 59, found %q", clock)
	}
	return h, m, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
