package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func setFlag(fs *flag.FlagSet, names ...string) string {
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	for _, n := range names {
		if given[n] {
			return n
		}
	}
	return ""
}

// parseSet turns repeated --set name=value into a map. A name given twice is
// an error: which one should win is not something to guess.
func parseSet(sets []string) (map[string]string, error) {
	out := make(map[string]string, len(sets))
	for _, kv := range sets {
		name, value, ok := strings.Cut(kv, "=")
		name = strings.TrimSpace(name)
		switch {
		case !ok || name == "":
			return nil, fmt.Errorf("--set %q: expected name=value", kv)
		case hasKey(out, name):
			return nil, fmt.Errorf("--set %s: given twice", name)
		}
		out[name] = value
	}
	return out, nil
}

func hasKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}

// argv prints a command so that it can be read back: an argument with a space
// is quoted, or "--team Small appliances" would look like two arguments when
// it is one.
func argv(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			a = strconv.Quote(a)
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}
