package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			// Surface panics on stderr so silent crashes don't leave Claude
			// Code with an empty statusline and no clue why.
			fmt.Fprintf(os.Stderr, "claude-gisx: panic: %v\n", r)
			os.Exit(2)
		}
	}()

	enableWinConsole() // no-op on non-Windows

	pos, flags := parseFlags(os.Args[1:])
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
			fmt.Println(version)
		case "setup":
			os.Exit(installCmd(installOpts{
				force:   flags["force"],
				noCheck: flags["no-check"],
			}))
		case "uninstall":
			os.Exit(uninstallCmd(installOpts{force: flags["force"]}))
		case "status":
			os.Exit(statusCmd())
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
			fmt.Fprintln(os.Stderr, "run 'claude-gisx help' for usage")
			os.Exit(1)
		}
		return
	}

	// No args: if stdin is piped, render from it; otherwise show help.
	if !stdinIsTerminal() {
		raw, err := io.ReadAll(os.Stdin)
		if err == nil && len(raw) > 0 {
			renderStatusline(string(raw))
			return
		}
	}
	helpScreen()
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
