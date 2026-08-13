package bash

import (
	"os/exec"
	"runtime"
)

// shell represents the shell that will be used to execute commands.
// dialect is the shell dialect to use (e.g. bash, pwsh, cmd). Dialect it will be injected into
// the Description() of the tool, so the model knows which shell to use.
type shell struct {
	path    string
	flag    string
	dialect string
}

func detectShell() shell {
	if p, err := exec.LookPath("bash"); err == nil {
		return shell{path: p, flag: "-c", dialect: "bash"}
	}

	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("pwsh"); err == nil {
			return shell{path: p, flag: "-Command", dialect: "powershell"}
		}

		if p, err := exec.LookPath("powershell"); err == nil {
			return shell{path: p, flag: "-Command", dialect: "powershell"}
		}

		return shell{path: "cmd", flag: "/c", dialect: "cmd.exe"}
	}

	if p, err := exec.LookPath("sh"); err == nil {
		return shell{path: p, flag: "-c", dialect: "sh (POSIX)"}
	}

	return shell{path: "sh", flag: "-c", dialect: "sh (POSIX)"}
}
