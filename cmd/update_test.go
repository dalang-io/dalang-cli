package cmd

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetBinaryName(t *testing.T) {
	name := getBinaryName()

	if !strings.HasPrefix(name, "dalang-") {
		t.Fatalf("expected binary name to start with 'dalang-', got %q", name)
	}

	if !strings.Contains(name, runtime.GOOS) {
		t.Fatalf("expected binary name to contain OS %q, got %q", runtime.GOOS, name)
	}

	if !strings.Contains(name, runtime.GOARCH) {
		t.Fatalf("expected binary name to contain arch %q, got %q", runtime.GOARCH, name)
	}

	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		t.Fatal("expected .exe suffix on Windows")
	}

	if runtime.GOOS != "windows" && strings.HasSuffix(name, ".exe") {
		t.Fatal("unexpected .exe suffix on non-Windows")
	}
}

func TestVersionInfoStruct(t *testing.T) {
	info := VersionInfo{
		Version: "v1.2.3",
	}

	if info.Version != "v1.2.3" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
}

func TestCmdUpdateHelp(t *testing.T) {
	err := cmdUpdate([]string{"--help"})
	if err != nil {
		t.Fatalf("cmdUpdate --help returned error: %v", err)
	}

	err = cmdUpdate([]string{"-h"})
	if err != nil {
		t.Fatalf("cmdUpdate -h returned error: %v", err)
	}
}
