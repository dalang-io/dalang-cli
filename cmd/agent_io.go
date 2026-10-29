package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// Plumbing for commands an AI agent drives without a dashboard: output that can be
// parsed, no prompt that can hang, and a bounded wait for an asynchronous result.
//
//   - stdout carries only the result. Under --json it is one JSON document; what a human
//     would read as progress goes to stderr (say), so `dalang ... --json | jq` always works.
//   - nothing that costs money or destroys data proceeds without --yes when there is no
//     terminal to ask (requireConfirmation): an agent that forgot --yes gets an error, not
//     a process waiting on a prompt nobody will answer.

// say prints progress for a human. Under --json it goes to stderr so stdout stays JSON.
func say(format string, args ...interface{}) {
	if quietOutput {
		return
	}
	out := io.Writer(os.Stdout)
	if jsonOutput {
		out = os.Stderr
	}
	fmt.Fprintf(out, format+"\n", args...)
}

// stderrOrStdout is where a human-readable header goes beside machine-readable output.
func stderrOrStdout() io.Writer {
	if jsonOutput {
		return os.Stderr
	}
	return os.Stdout
}

// emitJSON writes the result document to stdout.
func emitJSON(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

// A real terminal: os.ModeCharDevice alone is true for /dev/null too, which is exactly what
// an agent runs a command with.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// requireConfirmation: --yes goes ahead; a terminal gets the question; anything else
// is refused rather than left waiting.
func requireConfirmation(question string) error {
	if yesFlag {
		return nil
	}
	if !stdinIsTerminal() {
		return fmt.Errorf("refusing to continue without confirmation: pass --yes (%s)", question)
	}
	if !confirmPrompt(question) {
		return fmt.Errorf("cancelled")
	}
	return nil
}

// pollSleep is a seam: tests do not wait.
var pollSleep = time.Sleep

// waitFor calls check every `every` until it reports done, fails, or `timeout` passes.
func waitFor(timeout, every time.Duration, what string, check func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if time.Now().Add(every).After(deadline) {
			return fmt.Errorf("timed out after %s waiting for %s", timeout.Round(time.Second), what)
		}
		pollSleep(every)
	}
}

// kvFlag collects repeated --env KEY=VALUE.
type kvFlag map[string]string

func (k *kvFlag) String() string { return fmt.Sprint(map[string]string(*k)) }
func (k *kvFlag) Set(v string) error {
	i := strings.Index(v, "=")
	if i <= 0 {
		return fmt.Errorf("expected KEY=VALUE, got %q", v)
	}
	key := v[:i]
	for _, r := range key {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return fmt.Errorf("invalid variable name %q (letters, digits and _ only)", key)
		}
	}
	if *k == nil {
		*k = kvFlag{}
	}
	(*k)[key] = v[i+1:]
	return nil
}

// newFlags makes a FlagSet that returns errors instead of exiting and prints nothing
// itself: the dispatcher shows the error once.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseInterspersed parses flags that may come before or after positional arguments
// (`dalang app info shop --json` and `dalang app info --json shop` both work).
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}
