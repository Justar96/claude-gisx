package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/Justar96/claude-gisx/internal/app"
)

// The companion mod `setup` installs into ~/.claude/skills/gisx. `all:` keeps
// its .claude-plugin folder, which a plain pattern skips as hidden.
//
//go:embed all:mods/gisx
var modFS embed.FS

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

	// version is linker-injected into this package; propagate to app.
	app.Version = version
	if sub, err := fs.Sub(modFS, "mods/gisx"); err == nil {
		app.ModFS = sub
	}
	os.Exit(app.Run(os.Args[1:]))
}
