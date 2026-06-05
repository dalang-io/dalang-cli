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

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.7.3", "v1.7.2", true},      // patch bump
		{"v1.8.0", "v1.7.9", true},      // minor bump
		{"v2.0.0", "v1.9.9", true},      // major bump
		{"v1.7.2", "v1.7.2", false},     // equal
		{"v1.7.2", "v1.7.3", false},     // older
		{"1.7.3", "1.7.2", true},        // no "v" prefix
		{"v1.7.10", "v1.7.9", true},     // numeric, not lexical
		{"v1.7", "v1.7.0", false},       // shorter == longer when prefix equal
		{"v1.7.1", "v1.7", true},        // longer with extra component is newer
		{"v1.7.3", "dev", true},         // unparseable current -> update
		{"latest", "v1.7.2", true},      // unparseable latest, differs -> update
		{"v1.7.2-rc1", "v1.7.2", false}, // pre-release suffix stripped -> equal
	}
	for _, tc := range cases {
		if got := isNewerVersion(tc.latest, tc.current); got != tc.want {
			t.Errorf("isNewerVersion(%q,%q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	if nums, ok := parseVersion("v1.7.2"); !ok || len(nums) != 3 || nums[0] != 1 || nums[1] != 7 || nums[2] != 2 {
		t.Fatalf("parseVersion(v1.7.2) = %v, %v", nums, ok)
	}
	if _, ok := parseVersion("dev"); ok {
		t.Error("parseVersion(dev) should fail")
	}
	if _, ok := parseVersion(""); ok {
		t.Error("parseVersion(empty) should fail")
	}
	if nums, ok := parseVersion("1.7.2-rc1"); !ok || len(nums) != 3 {
		t.Errorf("parseVersion(1.7.2-rc1) = %v, %v", nums, ok)
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
