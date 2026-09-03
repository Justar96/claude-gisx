package app

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"
)

type gitState struct {
	branch string
	dirty  string
}

// One `git status` where there used to be three commands. Porcelain v2 carries
// the branch name in its header, and status already fails outside a work tree,
// so the separate rev-parse probe and symbolic-ref call were both redundant.
// Process spawns dominate this statusline's cost on a slow machine — cutting
// three to one is the single biggest win available.
//
// --no-optional-locks: a status that can't take index.lock (Claude Code or the
// user mid-commit) would otherwise fail and blank the branch. It also skips
// the index refresh write, so a render never contends with real git work.
// --no-ahead-behind: counting commits against upstream walks history, and the
// count isn't shown.
func gitInfo(cwd string) gitState {
	out, err := exec.Command("git", "--no-optional-locks", "-C", cwd, "status", "--porcelain=v2", "--branch", "--no-ahead-behind").Output()
	if err != nil {
		return gitState{}
	}
	var g gitState
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if b, ok := strings.CutPrefix(line, "# branch.head "); ok {
			// Matches the old symbolic-ref behavior, which errored out on a
			// detached HEAD and left the branch blank.
			if b = strings.TrimSpace(b); b != "(detached)" {
				g.branch = b
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Headers come first, so the first entry line proves the tree is dirty
		// and there's nothing left to learn — don't walk thousands of paths.
		if line != "" {
			g.dirty = "*"
			break
		}
	}
	return g
}
