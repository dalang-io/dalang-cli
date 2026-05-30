//go:build !windows

package cmd

// enableWindowsConsole is a no-op on non-Windows platforms: their terminals
// already speak UTF-8 and interpret ANSI escapes natively.
func enableWindowsConsole() {}
