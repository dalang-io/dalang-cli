//go:build !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/dalang-io/dalang-cli/internal/terminal"
)

func setupResizeHandler(sigChan chan os.Signal, term *terminal.Terminal) {
	resizeChan := make(chan os.Signal, 1)
	signal.Notify(resizeChan, syscall.SIGWINCH)

	go func() {
		for range resizeChan {
			term.SendResize()
		}
	}()
}
