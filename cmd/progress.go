package cmd

import (
	"fmt"
	"io"
	"os"
	"time"
)

// progressWriter renders a download progress bar as bytes flow through it.
// It satisfies io.Writer so it can be dropped into an io.MultiWriter alongside
// the file and hash writers. Rendering is throttled to avoid flicker.
type progressWriter struct {
	total      int64
	downloaded int64
	lastPrint  time.Time
	out        io.Writer // defaults to os.Stdout when nil (overridable in tests)
}

func (p *progressWriter) writer() io.Writer {
	if p.out != nil {
		return p.out
	}
	return os.Stdout
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n := len(b)
	p.downloaded += int64(n)
	if now := time.Now(); now.Sub(p.lastPrint) >= 80*time.Millisecond {
		p.lastPrint = now
		p.render()
	}
	return n, nil
}

func (p *progressWriter) render() {
	if p.total <= 0 {
		return
	}
	pct := float64(p.downloaded) / float64(p.total) * 100
	fmt.Fprintf(p.writer(), "\r  %s %3.0f%% (%s / %s)  ",
		renderBar(pct, 24), pct, formatBytes(p.downloaded), formatBytes(p.total))
}

// finish draws the final 100% state and moves to a new line.
func (p *progressWriter) finish() {
	p.downloaded = p.total
	p.render()
	fmt.Fprintln(p.writer())
}
