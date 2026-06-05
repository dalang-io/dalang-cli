package cmd

import (
	"fmt"
	"io"
	"os"
	"time"
)

// spinnerFrames are the Braille animation frames used by the spinner.
var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// spinner shows an animated indicator (on stderr by default) until Stop()ed,
// so long-running blocking calls don't look frozen. Output goes to stderr so it
// never pollutes stdout (which may be piped or parsed as JSON).
type spinner struct {
	label string
	out   io.Writer
	stop  chan struct{}
	done  chan struct{}
}

// startSpinner launches a spinner writing to stderr and returns it. Call Stop()
// to halt the animation and clear the line.
func startSpinner(label string) *spinner {
	return startSpinnerTo(os.Stderr, label)
}

// startSpinnerTo is startSpinner with an explicit writer (used in tests).
func startSpinnerTo(out io.Writer, label string) *spinner {
	s := &spinner{
		label: label,
		out:   out,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *spinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-s.stop:
			fmt.Fprint(s.out, "\r\033[K") // clear the spinner line
			return
		case <-ticker.C:
			fmt.Fprintf(s.out, "\r%s%c%s %s", colorCyan, spinnerFrames[i%len(spinnerFrames)], colorReset, s.label)
			i++
		}
	}
}

// Stop halts the spinner and waits for its goroutine to exit. Safe to call once.
func (s *spinner) Stop() {
	close(s.stop)
	<-s.done
}
