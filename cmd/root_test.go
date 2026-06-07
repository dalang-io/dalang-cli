package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func resetGlobalFlags() {
	jsonOutput = false
	quietOutput = false
	yesFlag = false
	noColorFlag = false
	VerboseOutput = false
}

func TestSuggestCommand(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"serivce", "service"}, // transposition
		{"shel", "shell"},      // missing char
		{"versionn", "version"},
		{"exce", "exec"},
		{"helpp", "help"},
		{"zzzzzzzzzzzz", ""}, // nothing close
		{"", ""},
	}
	for _, tc := range cases {
		if got := suggestCommand(tc.input); got != tc.want {
			t.Errorf("suggestCommand(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestApplyColorSettingsDisables(t *testing.T) {
	// Save and restore the color vars so this test doesn't leak into others.
	save := [7]string{colorReset, colorRed, colorGreen, colorYellow, colorBlue, colorCyan, colorBold}
	t.Cleanup(func() {
		colorReset, colorRed, colorGreen, colorYellow, colorBlue, colorCyan, colorBold =
			save[0], save[1], save[2], save[3], save[4], save[5], save[6]
		noColorFlag = false
	})

	noColorFlag = true
	applyColorSettings()
	if colorReset != "" || colorBold != "" || colorCyan != "" {
		t.Errorf("expected colors disabled with --no-color, got reset=%q bold=%q", colorReset, colorBold)
	}
}

func TestParseGlobalFlags(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	args := []string{"--json", "service", "--quiet", "list", "--yes", "--verbose", "extra"}
	remaining := parseGlobalFlags(args)

	if !jsonOutput {
		t.Fatal("expected jsonOutput to be true")
	}
	if !quietOutput {
		t.Fatal("expected quietOutput to be true")
	}
	if !yesFlag {
		t.Fatal("expected yesFlag to be true")
	}
	if !VerboseOutput {
		t.Fatal("expected VerboseOutput to be true")
	}

	got := strings.Join(remaining, " ")
	want := "service list extra"
	if got != want {
		t.Fatalf("unexpected remaining args: got %q want %q", got, want)
	}
}

func TestRenderBar(t *testing.T) {
	got := renderBar(150, 5)
	want := colorRed + "█████" + colorReset
	if got != want {
		t.Fatalf("unexpected bar: got %q want %q", got, want)
	}

	got = renderBar(70, 5)
	want = colorYellow + "███░░" + colorReset
	if got != want {
		t.Fatalf("unexpected warning bar: got %q want %q", got, want)
	}

	got = renderBar(20, 5)
	want = colorGreen + "█░░░░" + colorReset
	if got != want {
		t.Fatalf("unexpected normal bar: got %q want %q", got, want)
	}

	// Negative percentage should not panic (fix #12)
	got = renderBar(-10, 5)
	want = colorGreen + "░░░░░" + colorReset
	if got != want {
		t.Fatalf("unexpected negative bar: got %q want %q", got, want)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{name: "bytes", in: 999, want: "999 B"},
		{name: "kilobytes", in: 1024, want: "1 KB"},
		{name: "megabytes", in: 5 * 1024 * 1024, want: "5 MB"},
		{name: "gigabytes", in: 3 * 1024 * 1024 * 1024, want: "3.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatBytes(tt.in); got != tt.want {
				t.Fatalf("formatBytes(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCmdHelpForUnknownCommand(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	if err := cmdHelpFor("missing"); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestPrintHelpIncludesCoreSections(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe setup failed: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	printHelp()

	if err := w.Close(); err != nil {
		t.Fatalf("closing writer failed: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading output failed: %v", err)
	}

	out := buf.String()
	for _, needle := range []string{
		"Dalang CLI",
		"COMMANDS:",
		"service list",
		"scp [-r] <src>... <dst>",
		"https://dalang.io/docs/cli",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("help output missing %q", needle)
		}
	}
}
