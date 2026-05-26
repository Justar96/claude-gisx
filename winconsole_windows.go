//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// enableWinConsole sets the console output code page to UTF-8 and enables
// ANSI VT processing on stdout/stderr. Without this, the box-drawing chars
// (█, ░) render as mojibake and the ANSI escapes show up as literal `␛[31m`.
//
// Pure stdlib via direct kernel32 syscalls — no x/sys/windows dependency.
func enableWinConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")

	const cpUTF8 = 65001
	const enableVTProcessing = 0x0004

	_, _, _ = setConsoleOutputCP.Call(uintptr(cpUTF8))

	enableVT := func(h syscall.Handle) {
		var mode uint32
		r, _, _ := getConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode)))
		if r == 0 {
			return
		}
		mode |= enableVTProcessing
		_, _, _ = setConsoleMode.Call(uintptr(h), uintptr(mode))
	}
	if h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil {
		enableVT(h)
	}
	if h, err := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE); err == nil {
		enableVT(h)
	}
}
