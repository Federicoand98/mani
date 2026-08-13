package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

func versionString() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "mani (unknown version)"
	}

	v := bi.Main.Version
	var rev string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}

	if (v == "" || v == "(devel)") && rev != "" {
		v = rev
		if dirty {
			v += "-dirty"
		}
	}
	if v == "" {
		v = "devel"
	}

	return fmt.Sprintf("mani %s %s/%s (go %s)", v, runtime.GOOS, runtime.GOARCH, bi.GoVersion)
}
