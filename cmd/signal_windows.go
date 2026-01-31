//go:build windows

package cmd

import (
	"os"

	"github.com/dalang-io/dalang-cli/internal/terminal"
)

func setupResizeHandler(sigChan chan os.Signal, term *terminal.Terminal) {
	// Windows doesn't support SIGWINCH
	// Terminal resize is handled differently on Windows
}
