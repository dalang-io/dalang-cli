//go:build windows

package cmd

import "os"

// tunnelStopSignals are the signals that end a tunnel cleanly. Windows only
// delivers os.Interrupt (Ctrl+C / Ctrl+Break); SIGTERM exists in the syscall
// package but is never sent, so listing it would be noise.
func tunnelStopSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
