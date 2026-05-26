package app

import (
	"fmt"
	"io"
	"os"
)

// Version is set from the root main package at startup; the linker writes
// the release tag into main.version via -X, and main copies it across.
var Version = "dev"

// Run is the CLI entry point. It returns the exit code instead of calling
// os.Exit so panic-recovery and post-run cleanup live in the root main.
func Run(args []string) int {
	pos, flags := parseFlags(args)
	cmd := ""
	if len(pos) > 0 {
		cmd = pos[0]
	}

	// A subcommand wins outright — never touch stdin (which can block when
	// the caller hands us an inherited pipe, e.g. curl | bash).
	if cmd != "" {
		switch cmd {
		case "help", "--help", "-h":
			helpScreen()
		case "version", "--version", "-v":
			fmt.Println(Version)
		case "setup":
			return installCmd(installOpts{
				force:   flags["force"],
				noCheck: flags["no-check"],
			})
		case "uninstall":
			return uninstallCmd(installOpts{force: flags["force"]})
		case "status":
			return statusCmd()
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
			fmt.Fprintln(os.Stderr, "run 'claude-gisx help' for usage")
			return 1
		}
		return 0
	}

	// No args: if stdin is piped, render from it; otherwise show help.
	if !stdinIsTerminal() {
		raw, err := io.ReadAll(os.Stdin)
		if err == nil && len(raw) > 0 {
			renderStatusline(string(raw))
			return 0
		}
	}
	helpScreen()
	return 0
}

func parseFlags(args []string) (positional []string, flags map[string]bool) {
	flags = map[string]bool{}
	for _, a := range args {
		if len(a) > 2 && a[:2] == "--" {
			flags[a[2:]] = true
		} else {
			positional = append(positional, a)
		}
	}
	return
}

func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
