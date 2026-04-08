package cmd

import "testing"

func TestCmdStartNoArgs(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdStart([]string{})
	if err == nil {
		t.Fatal("cmdStart with no args should return error")
	}
}

func TestCmdStopNoArgs(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdStop([]string{})
	if err == nil {
		t.Fatal("cmdStop with no args should return error")
	}
}

func TestCmdDeleteNoArgs(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdDelete([]string{})
	if err == nil {
		t.Fatal("cmdDelete with no args should return error")
	}
}

func TestCmdStartHelp(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdStart([]string{"--help"})
	if err != nil {
		t.Fatalf("cmdStart --help returned error: %v", err)
	}

	err = cmdStart([]string{"-h"})
	if err != nil {
		t.Fatalf("cmdStart -h returned error: %v", err)
	}
}

func TestCmdStopHelp(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdStop([]string{"--help"})
	if err != nil {
		t.Fatalf("cmdStop --help returned error: %v", err)
	}
}

func TestCmdDeleteHelp(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdDelete([]string{"--help"})
	if err != nil {
		t.Fatalf("cmdDelete --help returned error: %v", err)
	}
}
