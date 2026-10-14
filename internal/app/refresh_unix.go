//go:build !windows

package app

import "syscall"

// A new session keeps the child alive when Claude Code reaps the
// statusline's process group.
func detachedAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
