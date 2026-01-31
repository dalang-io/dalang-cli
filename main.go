package main

import (
	"fmt"
	"os"

	"github.com/dalang-io/dalang-cli/cmd"
)

// Version info - set via ldflags at build time
var (
	Version   = "dev"
	BuildDate = "unknown"
	Commit    = "unknown"
)

func main() {
	// Pass version info to cmd package
	cmd.Version = Version
	cmd.BuildDate = BuildDate
	cmd.Commit = Commit

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
