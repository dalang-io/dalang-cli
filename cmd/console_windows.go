//go:build windows

package cmd

import (
	"golang.org/x/sys/windows"
)

// enableWindowsConsole makes the legacy Windows console (conhost / PowerShell 5.1)
// render the CLI's UTF-8 glyphs and ANSI colors correctly.
//
// Two separate problems show up as "broken characters" on Windows:
//
//  1. Mojibake (ΓöÇ, ΓåÆ, Γûê ...): the console output code page defaults to an
//     OEM page (e.g. 437/850), so the UTF-8 bytes we print for ─ → █ ░ ✓ ✗ are
//     decoded with the wrong table. SetConsoleOutputCP(CP_UTF8) fixes decoding.
//
//  2. Literal escapes (←[32m ... ←[0m): ANSI sequences (\033[..m) are printed
//     verbatim unless the console has ENABLE_VIRTUAL_TERMINAL_PROCESSING.
//     Modern Windows Terminal enables it; the classic console does not.
//
// Best-effort: every step is allowed to fail silently — the CLI still works,
// it just falls back to the same un-prettified output it had before.
func enableWindowsConsole() {
	const cpUTF8 = 65001

	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	if p := kernel32.NewProc("SetConsoleOutputCP"); p.Find() == nil {
		_, _, _ = p.Call(uintptr(cpUTF8))
	}
	if p := kernel32.NewProc("SetConsoleCP"); p.Find() == nil {
		_, _, _ = p.Call(uintptr(cpUTF8))
	}

	for _, std := range []uint32{windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
		handle, err := windows.GetStdHandle(std)
		if err != nil {
			continue
		}
		var mode uint32
		if err := windows.GetConsoleMode(handle, &mode); err != nil {
			continue
		}
		_ = windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
