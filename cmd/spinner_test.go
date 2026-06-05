package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinnerStartStop(t *testing.T) {
	var buf bytes.Buffer
	s := startSpinnerTo(&buf, "Running...")

	// Allow at least one animation frame (ticker is 80ms). Only the spinner
	// goroutine touches buf during the sleep; the test reads it after Stop()
	// joins the goroutine, so there is no concurrent access.
	time.Sleep(120 * time.Millisecond)
	s.Stop() // must return (join the goroutine), not hang

	out := buf.String()
	if !strings.Contains(out, "Running...") {
		t.Errorf("spinner output missing label: %q", out)
	}
	if !strings.Contains(out, "\033[K") {
		t.Errorf("spinner should clear its line on Stop: %q", out)
	}
}

func TestSpinnerFramesNonEmpty(t *testing.T) {
	if len(spinnerFrames) == 0 {
		t.Fatal("spinnerFrames must not be empty")
	}
}
