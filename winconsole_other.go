//go:build !windows

package main

// enableWinConsole is a Windows-only console setup hook. On other platforms
// terminals already speak UTF-8 and ANSI, so this is a no-op.
func enableWinConsole() {}
