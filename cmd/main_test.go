package cmd

import (
	"os"
	"testing"
)

// TestMain isolates HOME for the entire cmd test package so that no test can
// ever read, overwrite, or delete the developer's real ~/.dalang credentials
// (e.g. via authLogout -> DeleteCredentials, or auto-refresh -> SaveCredentials).
// Individual tests may still call t.Setenv("HOME", t.TempDir()) for per-test
// isolation; this is the package-wide safety net.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "dalang-cli-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", tmp)        // darwin/linux
	os.Setenv("USERPROFILE", tmp) // windows
	os.Setenv("DALANG_API_URL", "")

	code := m.Run()

	os.RemoveAll(tmp)
	os.Exit(code)
}
