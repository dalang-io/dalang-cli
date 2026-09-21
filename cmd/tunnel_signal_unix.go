//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

// tunnelStopSignals are the signals that end a tunnel cleanly. SIGTERM matters
// here because a tunnel is long-lived and often ends up under a supervisor or
// in a container, which terminates rather than interrupts. syscall lives behind
// this build tag so shared files stay portable (Android/Termux, Windows).
func tunnelStopSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
