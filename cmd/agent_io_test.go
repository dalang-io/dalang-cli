package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestKVFlag(t *testing.T) {
	var k kvFlag
	for _, v := range []string{"PORT=3000", "DATABASE_URL=postgres://u:p@h/db?x=1", "EMPTY="} {
		if err := k.Set(v); err != nil {
			t.Fatalf("%q: %v", v, err)
		}
	}
	if k["DATABASE_URL"] != "postgres://u:p@h/db?x=1" || k["EMPTY"] != "" || k["PORT"] != "3000" {
		t.Errorf("parsed %v (a value may contain = and may be empty)", map[string]string(k))
	}
	for _, bad := range []string{"NOEQUALS", "=value", "BAD-NAME=1", "A B=1"} {
		var x kvFlag
		if err := x.Set(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseInterspersed(t *testing.T) {
	var months int
	var wait bool
	fs := newFlags("t")
	fs.IntVar(&months, "months", 1, "")
	fs.BoolVar(&wait, "wait", false, "")
	pos, err := parseInterspersed(fs, []string{"shop", "--months", "12", "--wait"})
	if err != nil || len(pos) != 1 || pos[0] != "shop" || months != 12 || !wait {
		t.Errorf("flags after the positional: %v %v months=%d wait=%v", pos, err, months, wait)
	}
	fs = newFlags("t")
	fs.IntVar(&months, "months", 1, "")
	if _, err := parseInterspersed(fs, []string{"--nope"}); err == nil {
		t.Error("an unknown flag must be an error, not ignored")
	}
}

func TestRequireConfirmationRefusesWithoutAnswerableTerminal(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	yesFlag = false
	// `go test` has no terminal on stdin: an agent that forgot --yes must get an error.
	if err := requireConfirmation("Pay Rp 1?"); err == nil || !strings.Contains(err.Error(), "pass --yes") {
		t.Errorf("without --yes and without a terminal: %v", err)
	}
	yesFlag = true
	if err := requireConfirmation("Pay Rp 1?"); err != nil {
		t.Errorf("with --yes: %v", err)
	}
}

func TestWaitFor(t *testing.T) {
	var slept []time.Duration
	pollSleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { pollSleep = time.Sleep })

	n := 0
	err := waitFor(time.Minute, time.Second, "x", func() (bool, error) { n++; return n == 3, nil })
	if err != nil || n != 3 || len(slept) != 2 {
		t.Errorf("done on the third look: err=%v calls=%d sleeps=%d", err, n, len(slept))
	}

	boom := errors.New("boom")
	if err := waitFor(time.Minute, time.Second, "x", func() (bool, error) { return false, boom }); err != boom {
		t.Errorf("a failure ends the wait: %v", err)
	}

	err = waitFor(3*time.Second, time.Second, "the app to run", func() (bool, error) { return false, nil })
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "the app to run") {
		t.Errorf("never done: %v", err)
	}
}
