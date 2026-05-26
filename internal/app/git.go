package app

import (
	"os/exec"
	"strings"
)

type gitState struct {
	branch string
	dirty  string
}

func gitInfo(cwd string) gitState {
	if err := exec.Command("git", "-C", cwd, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return gitState{}
	}
	var g gitState
	if out, err := exec.Command("git", "-C", cwd, "symbolic-ref", "--short", "HEAD").Output(); err == nil {
		g.branch = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "-C", cwd, "status", "--porcelain").Output(); err == nil {
		if strings.TrimSpace(string(out)) != "" {
			g.dirty = "*"
		}
	}
	return g
}
