package main

import (
	"flag"
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
