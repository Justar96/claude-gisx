package main

import (
	"fmt"
	"os"

	"github.com/Justar96/claude-gisx/internal/app"
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

	// version is linker-injected into this package; propagate to app.
	app.Version = version
	os.Exit(app.Run(os.Args[1:]))
}
