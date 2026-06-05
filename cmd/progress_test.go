package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestProgressWriterAccumulatesAndFinishes(t *testing.T) {
	var buf bytes.Buffer
	p := &progressWriter{total: 1000, out: &buf}

	n, err := p.Write(make([]byte, 400))
	if err != nil || n != 400 {
		t.Fatalf("Write = %d, %v; want 400, nil", n, err)
	}
	if _, err := p.Write(make([]byte, 600)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if p.downloaded != 1000 {
		t.Fatalf("downloaded = %d, want 1000", p.downloaded)
	}

	p.finish()
	out := buf.String()
	if !strings.Contains(out, "100%") {
		t.Errorf("finish output missing 100%%: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("finish should end with a newline: %q", out)
	}
}

func TestProgressWriterZeroTotalRendersNothing(t *testing.T) {
	var buf bytes.Buffer
	p := &progressWriter{total: 0, out: &buf}
	p.render() // unknown size -> no bar
	if buf.Len() != 0 {
		t.Errorf("render with zero total should print nothing, got %q", buf.String())
	}
}
