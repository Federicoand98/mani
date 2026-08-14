package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var version string

func versionString() string {
	bi, _ := debug.ReadBuildInfo()

	v := version
	if v == "" {
		v = versionFromBuildInfo(bi)
	}

	goVer := runtime.Version()
	if bi != nil && bi.GoVersion != "" {
		goVer = bi.GoVersion
	}

	return fmt.Sprintf("mani %s %s/%s (%s)", v, runtime.GOOS, runtime.GOARCH, goVer)
}

func versionFromBuildInfo(bi *debug.BuildInfo) string {
	if bi == nil {
		return "unknown"
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

	if v == "" || v == "(devel)" {
		if rev == "" {
			return "devel"
		}
		v = rev
		if dirty {
			v += "-dirty"
		}
	}

	return v
}
