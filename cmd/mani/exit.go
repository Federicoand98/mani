package main

import (
	"errors"
	"fmt"
	"os"
)

const (
	exitOK      = 0
	exitRuntime = 1
	exitUsage   = 2
)

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

func usagef(format string, args ...any) error {
	return usageError{fmt.Errorf(format, args...)}
}

func exitCodeFor(err error) int {
	var ue usageError
	if errors.As(err, &ue) {
		return exitUsage
	}
	return exitRuntime
}

func fail(code int, context string, err error) {
	fmt.Fprintf(os.Stderr, "mani %s: %v\n", context, err)
	os.Exit(code)
}
