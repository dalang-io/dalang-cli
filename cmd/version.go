package cmd

import (
	"encoding/json"
	"fmt"
)

func cmdVersion() error {
	if jsonOutput {
		output := map[string]string{
			"version":    Version,
			"build_date": BuildDate,
			"commit":     Commit,
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("%sDalang CLI%s version %s%s%s", colorBold, colorReset, colorCyan, Version, colorReset)
	if BuildDate != "unknown" || Commit != "unknown" {
		fmt.Printf(" (build %s, commit %s)", BuildDate, Commit)
	}
	fmt.Println()
	return nil
}
